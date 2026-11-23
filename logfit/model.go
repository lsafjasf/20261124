package main

import "math"

// 统一 Log 曲线模型（码值域归一化到 0..1，线性域 0.18 = 中灰）：
//
//	L >= t:  V(L) = A*log10(L+B) + C        (log 段，含肩部滚降)
//	L <  t:  V(L) = S*L + K                 (线性趾部)
//
// 在截点 t 处强制 C0 连续：K = A*log10(t+B) + C - S*t。
// 自由参数 theta = (A, B, C, t, S)，三种风格仅先验中心不同。
// 该形式可精确复现 S-Log3 / V-Log / LogC(EI800) 官方曲线。
type Model struct {
	Name  string
	Canon [5]float64 // 典型参数 (A, B, C, t, S)，用作 MAP 先验中心
}

var models = map[string]Model{
	// S-Log3: V=(420+261.5*log10((L+0.01)/0.19))/1023, 趾部 (0,95)->(0.01125,171.21)/1023
	"slog3": {"S-Log3", [5]float64{0.2556207, 0.01, 0.594922, 0.01125, 6.621943}},
	// V-Log: V=0.241514*log10(L+0.00873)+0.598206, 趾部 5.6*L+0.125, cut=0.01
	"vlog": {"V-Log", [5]float64{0.241514, 0.00873, 0.598206, 0.01, 5.6}},
	// LogC EI800: V=0.247190*log10(5.555556*L+0.052272)+0.385537, 趾部 5.367655*L+0.092809
	"logc": {"LogC", [5]float64{0.247190, 0.009409, 0.569654, 0.010591, 5.367655}},
}

const (
	grayRef     = 0.18 // 中灰反射率
	shoulderRef = 0.9  // 肩部参考：90% 反射率白卡
)

// codeValue 计算参数 p 下、线性亮度 L 对应的归一化码值。
func codeValue(p [5]float64, L float64) float64 {
	A, B, C, t, S := p[0], p[1], p[2], p[3], p[4]
	if L >= t {
		return A*math.Log10(L+B) + C
	}
	K := A*math.Log10(t+B) + C - S*t
	return S*L + K
}

// derived 由参数推出技术员关心的三个工作点。
func derived(p [5]float64) (black, gray, shoulder float64) {
	black = codeValue(p, 0)
	gray = codeValue(p, grayRef)
	shoulder = codeValue(p, shoulderRef)
	return
}

// ---- 无约束化参数变换：优化在 phi 空间进行 ----
// phi = (A, ln B, C, logit(t/tMax), ln S)

const tMax = 0.06 // 截点上界，覆盖三种风格的 cut

func toPhi(p [5]float64) []float64 {
	u := p[3] / tMax
	return []float64{
		p[0],
		math.Log(p[1]),
		p[2],
		math.Log(u / (1 - u)),
		math.Log(p[4]),
	}
}

func fromPhi(phi []float64) [5]float64 {
	u := 1 / (1 + math.Exp(-phi[3]))
	return [5]float64{
		phi[0],
		math.Exp(phi[1]),
		phi[2],
		tMax * u,
		math.Exp(phi[4]),
	}
}

// priorWidths 为 phi 空间先验宽度（岭正则的尺度），
// 取得较宽：数据充分时数据主导，样本稀少时防止参数发散。
var priorWidths = [5]float64{0.15, 1.0, 0.2, 1.2, 0.8}
