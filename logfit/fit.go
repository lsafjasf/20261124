package main

import "math"

// Sample 为一次记录：线性亮度 L 与实测归一化码值 V。
type Sample struct {
	L, V float64
}

// FitResult 保存一次 MAP 拟合的结果。
type FitResult struct {
	Model  Model
	Phi    []float64  // 最优点（phi 空间）
	Params [5]float64 // 最优点（物理参数）
	SSE    float64    // 数据残差平方和（不含先验）
	Resid  []float64  // 每点残差 V_meas - V_fit
	Sigma  float64    // 噪声估计 sqrt(SSE/(n-p))，n<=p 时退化为 sqrt(SSE/n)
	N, P   int
}

const huberDelta = 0.01 // 码值域 Huber 拐点，抑制野值

func huber(r float64) float64 {
	a := math.Abs(r)
	if a <= huberDelta {
		return 0.5 * r * r
	}
	return huberDelta * (a - 0.5*huberDelta)
}

// objective 构造 MAP 目标：Huber 数据项 + 以典型值为中心的岭先验。
// 返回的函数同时给出数据 SSE（供剖面分析使用）。
func objective(m Model, data []Sample) (cost func(phi []float64) float64, dataSSE func(phi []float64) float64) {
	center := toPhi(m.Canon)
	penalty := func(phi []float64) float64 {
		s := 0.0
		for j := range phi {
			d := (phi[j] - center[j]) / priorWidths[j]
			s += d * d
		}
		return s
	}
	dataSSE = func(phi []float64) float64 {
		p := fromPhi(phi)
		s := 0.0
		for _, d := range data {
			r := d.V - codeValue(p, d.L)
			s += r * r
		}
		return s
	}
	cost = func(phi []float64) float64 {
		p := fromPhi(phi)
		s := penalty(phi)
		for _, d := range data {
			s += huber(d.V - codeValue(p, d.L))
		}
		return s
	}
	return
}

// fit 从典型值出发做 Nelder-Mead，返回拟合结果。
func fit(m Model, data []Sample, start []float64) FitResult {
	cost, _ := objective(m, data)
	if start == nil {
		start = toPhi(m.Canon)
	}
	step := make([]float64, len(start))
	for j := range step {
		step[j] = 0.25 * priorWidths[j]
	}
	phi := nelderMead(cost, start, step, 4000, 1e-12)
	p := fromPhi(phi)
	res := FitResult{Model: m, Phi: phi, Params: p, N: len(data), P: 5}
	for _, d := range data {
		res.Resid = append(res.Resid, d.V-codeValue(p, d.L))
		res.SSE += res.Resid[len(res.Resid)-1] * res.Resid[len(res.Resid)-1]
	}
	dof := res.N - res.P
	if dof < 1 {
		dof = 1 // 样本不足时退化为有偏估计，并在报告中告警
	}
	res.Sigma = math.Sqrt(res.SSE / float64(dof))
	return res
}

// nelderMead 为经典单纯形法（仅依赖 math）。
func nelderMead(f func([]float64) float64, x0, step []float64, maxIter int, tol float64) []float64 {
	n := len(x0)
	simplex := make([][]float64, n+1)
	vals := make([]float64, n+1)
	for i := 0; i <= n; i++ {
		simplex[i] = append([]float64(nil), x0...)
		if i > 0 {
			simplex[i][i-1] += step[i-1]
		}
		vals[i] = f(simplex[i])
	}
	centroid := make([]float64, n)
	for iter := 0; iter < maxIter; iter++ {
		// 排序：升序
		for i := 1; i <= n; i++ {
			for k := i; k > 0 && vals[k] < vals[k-1]; k-- {
				vals[k], vals[k-1] = vals[k-1], vals[k]
				simplex[k], simplex[k-1] = simplex[k-1], simplex[k]
			}
		}
		spread := math.Abs(vals[n]-vals[0]) / (1 + math.Abs(vals[0]))
		if spread < tol {
			break
		}
		for j := 0; j < n; j++ {
			centroid[j] = 0
			for i := 0; i < n; i++ {
				centroid[j] += simplex[i][j]
			}
			centroid[j] /= float64(n)
		}
		try := func(alpha float64) ([]float64, float64) {
			x := make([]float64, n)
			for j := 0; j < n; j++ {
				x[j] = centroid[j] + alpha*(centroid[j]-simplex[n][j])
			}
			return x, f(x)
		}
		xr, fr := try(1) // 反射
		switch {
		case fr < vals[0]:
			xe, fe := try(2) // 扩展
			if fe < fr {
				simplex[n], vals[n] = xe, fe
			} else {
				simplex[n], vals[n] = xr, fr
			}
		case fr < vals[n-1]:
			simplex[n], vals[n] = xr, fr
		default:
			xc, fc := try(-0.5) // 收缩
			if fc < vals[n] {
				simplex[n], vals[n] = xc, fc
			} else { // 整体压缩
				for i := 1; i <= n; i++ {
					for j := 0; j < n; j++ {
						simplex[i][j] = simplex[0][j] + 0.5*(simplex[i][j]-simplex[0][j])
					}
					vals[i] = f(simplex[i])
				}
			}
		}
	}
	best := 0
	for i := 1; i <= n; i++ {
		if vals[i] < vals[best] {
			best = i
		}
	}
	return append([]float64(nil), simplex[best]...)
}

// looRMSE 留一交叉验证：每次剔除一点重新拟合，评估过拟合风险。
func looRMSE(m Model, data []Sample, full FitResult) float64 {
	sum := 0.0
	for i := range data {
		sub := make([]Sample, 0, len(data)-1)
		sub = append(sub, data[:i]...)
		sub = append(sub, data[i+1:]...)
		r := fit(m, sub, full.Phi) // 从全数据解热启动
		e := data[i].V - codeValue(r.Params, data[i].L)
		sum += e * e
	}
	return math.Sqrt(sum / float64(len(data)))
}

// aicc 小样本修正的 AIC（基于高斯假设，p 取全部 5 个自由参数，偏保守）。
func aicc(r FitResult) float64 {
	n, p := float64(r.N), float64(r.P)
	if r.SSE <= 0 {
		return math.Inf(-1)
	}
	correction := 2 * p * (p + 1) / (n - p - 1)
	if n-p-1 <= 0 {
		correction = math.Inf(1)
	}
	return n*math.Log(r.SSE/n) + 2*p + correction
}
