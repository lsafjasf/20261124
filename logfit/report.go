package main

import (
	"math"
)

// Interval 表示一个可能单侧无界的区间（无界侧以 Inf 表示）。
type Interval struct {
	Lo, Hi         float64
	LoFree, HiFree bool // true = 该侧在扫描范围内未越阈，参数不可辨识
}

// jacobian 用最优点处的中心差分计算 dV/dphi（n x p）。
func jacobian(r FitResult, data []Sample) [][]float64 {
	p := len(r.Phi)
	J := make([][]float64, len(data))
	for i, d := range data {
		J[i] = make([]float64, p)
		for j := 0; j < p; j++ {
			h := 1e-6 * (1 + math.Abs(r.Phi[j]))
			up := append([]float64(nil), r.Phi...)
			dn := append([]float64(nil), r.Phi...)
			up[j] += h
			dn[j] -= h
			J[i][j] = (codeValue(fromPhi(up), d.L) - codeValue(fromPhi(dn), d.L)) / (2 * h)
		}
	}
	return J
}

// invert 高斯-约当部分主元求逆；奇异时返回 false。
func invert(a [][]float64) ([][]float64, bool) {
	n := len(a)
	m := make([][]float64, n)
	for i := range a {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		piv := col
		for i := col + 1; i < n; i++ {
			if math.Abs(m[i][col]) > math.Abs(m[piv][col]) {
				piv = i
			}
		}
		if math.Abs(m[piv][col]) < 1e-14 {
			return nil, false
		}
		m[col], m[piv] = m[piv], m[col]
		d := m[col][col]
		for j := 0; j < 2*n; j++ {
			m[col][j] /= d
		}
		for i := 0; i < n; i++ {
			if i == col {
				continue
			}
			f := m[i][col]
			for j := 0; j < 2*n; j++ {
				m[i][j] -= f * m[col][j]
			}
		}
	}
	inv := make([][]float64, n)
	for i := range m {
		inv[i] = m[i][n:]
	}
	return inv, true
}

// derivedErrors 用 delta 法估计黑点/中灰/肩部的 1-sigma 误差。
// 协方差取 sigma^2 * (J^T J + R)^{-1}，R 为先验精度（岭项），
// 与 MAP 目标一致；J^T J 奇异时标记为不可辨识。
func derivedErrors(r FitResult, data []Sample) (vals, errs [3]float64, ok bool) {
	J := jacobian(r, data)
	p := len(r.Phi)
	jtj := make([][]float64, p)
	for i := range jtj {
		jtj[i] = make([]float64, p)
	}
	for _, row := range J {
		for a := 0; a < p; a++ {
			for b := 0; b < p; b++ {
				jtj[a][b] += row[a] * row[b]
			}
		}
	}
	for j := 0; j < p; j++ { // 先验精度（与 MAP 岭项同尺度）
		jtj[j][j] += 1 / (priorWidths[j] * priorWidths[j])
	}
	cov, ok := invert(jtj)
	if !ok {
		return vals, errs, false
	}
	black, gray, shoulder := derived(r.Params)
	vals = [3]float64{black, gray, shoulder}
	refs := [3]float64{0, grayRef, shoulderRef}
	for k := 0; k < 3; k++ {
		g := make([]float64, p) // d(derived_k)/dphi
		for j := 0; j < p; j++ {
			h := 1e-6 * (1 + math.Abs(r.Phi[j]))
			up := append([]float64(nil), r.Phi...)
			dn := append([]float64(nil), r.Phi...)
			up[j] += h
			dn[j] -= h
			g[j] = (codeValue(fromPhi(up), refs[k]) - codeValue(fromPhi(dn), refs[k])) / (2 * h)
		}
		v := 0.0
		for a := 0; a < p; a++ {
			for b := 0; b < p; b++ {
				v += g[a] * cov[a][b] * g[b]
			}
		}
		errs[k] = r.Sigma * math.Sqrt(math.Max(v, 0))
	}
	return vals, errs, true
}

// profileInterval 对第 j 个 phi 参数做剖面 SSE 扫描：
// 固定该参数、重优化其余参数，记录数据 SSE 超过 SSE_min + sigma^2
// （即 Δχ²=1，约 68% 置信）的边界。扫描到边界仍未越阈则该侧
// 标记为不可辨识（数据无法区分该方向上的参数变化）。
func profileInterval(m Model, data []Sample, r FitResult, j int) Interval {
	const nGrid = 40
	threshold := r.SSE + r.Sigma*r.Sigma
	_, dataSSE := objective(m, data)
	span := 3 * priorWidths[j]
	iv := Interval{Lo: math.Inf(-1), Hi: math.Inf(1), LoFree: true, HiFree: true}
	for side := -1; side <= 1; side += 2 {
		for k := 1; k <= nGrid; k++ {
			off := float64(side) * float64(k) / float64(nGrid) * span
			fixed := r.Phi[j] + off
			// 固定第 j 维，优化其余维
			rest := make([]int, 0, len(r.Phi)-1)
			for q := range r.Phi {
				if q != j {
					rest = append(rest, q)
				}
			}
			start := make([]float64, len(rest))
			step := make([]float64, len(rest))
			for i, q := range rest {
				start[i] = r.Phi[q]
				step[i] = 0.25 * priorWidths[q]
			}
			cost, _ := objective(m, data)
			sub := func(x []float64) float64 {
				full := append([]float64(nil), r.Phi...)
				full[j] = fixed
				for i, q := range rest {
					full[q] = x[i]
				}
				return cost(full)
			}
			opt := nelderMead(sub, start, step, 2000, 1e-12)
			full := append([]float64(nil), r.Phi...)
			full[j] = fixed
			for i, q := range rest {
				full[q] = opt[i]
			}
			if dataSSE(full) > threshold {
				if side < 0 {
					iv.Lo, iv.LoFree = fixed, false
				} else {
					iv.Hi, iv.HiFree = fixed, false
				}
				break
			}
		}
	}
	return iv
}
