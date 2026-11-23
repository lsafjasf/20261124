package main

import (
	"fmt"
	"math"
)

func printReport(m Model, data []Sample, r FitResult, truth *[5]float64) {
	fmt.Printf("== Log 曲线拟合报告 ==\n模型: %s   样本数 n=%d   自由参数 p=%d\n\n", m.Name, r.N, r.P)

	fmt.Println("-- 拟合参数（68% 剖面区间，* 表示该侧不可辨识）--")
	for j := 0; j < 5; j++ {
		iv := profileInterval(m, data, r, j)
		fmt.Printf("  %-12s = %10.5f   [%s, %s]\n",
			paramNames[j], r.Params[j],
			fmtBound(toPhys(r.Phi, j, iv.Lo), iv.LoFree),
			fmtBound(toPhys(r.Phi, j, iv.Hi), iv.HiFree))
	}

	fmt.Println("\n-- 导出工作点（delta 法 ±1σ）--")
	vals, errs, ok := derivedErrors(r, data)
	names := [3]string{"黑点 V(0)", "中灰 V(0.18)", "肩部 V(0.9)"}
	var tv [3]float64
	if truth != nil {
		tv[0], tv[1], tv[2] = derived(*truth)
	}
	for k := 0; k < 3; k++ {
		if !ok {
			fmt.Printf("  %-14s = %8.5f   (J^T J 奇异，误差不可估)\n", names[k], vals[k])
			continue
		}
		line := fmt.Sprintf("  %-14s = %8.5f ± %.5f", names[k], vals[k], errs[k])
		if truth != nil {
			line += fmt.Sprintf("   (真值 %.5f, 偏差 %+.5f)", tv[k], vals[k]-tv[k])
		}
		fmt.Println(line)
	}

	fmt.Println("\n-- 残差（σ = 噪声估计）--")
	fmt.Printf("  σ = %.5f\n", r.Sigma)
	fmt.Println("   #        L        V实测     V拟合     残差      标准化")
	for i, d := range data {
		vf := d.V - r.Resid[i]
		fmt.Printf("  %2d  %9.4f  %8.5f  %8.5f  %+8.5f  %+6.2f\n",
			i+1, d.L, d.V, vf, r.Resid[i], r.Resid[i]/math.Max(r.Sigma, 1e-12))
	}

	fmt.Println("\n-- 过拟合诊断 --")
	trainRMSE := math.Sqrt(r.SSE / float64(r.N))
	loo := looRMSE(m, data, r)
	fmt.Printf("  训练 RMSE = %.5f   留一 CV RMSE = %.5f   AICc = %.2f\n", trainRMSE, loo, aicc(r))
	if r.N <= r.P {
		fmt.Println("  ! 警告: n <= p，参数完全由先验支撑，区间仅供参考，请增加采样点")
	} else if r.N < 2*r.P {
		fmt.Println("  ! 提示: n < 2p，先验对结果影响较大，建议补采样（尤其趾部与肩部）")
	}
	if loo > 2*trainRMSE {
		fmt.Println("  ! 警告: 留一误差远大于训练误差，存在过拟合迹象")
	}
}

func fmtBound(v float64, free bool) string {
	if free {
		if v < 0 {
			return "  -∞(*)"
		}
		return "  +∞(*)"
	}
	return fmt.Sprintf("%9.5f", v)
}

// toPhys 把第 j 个 phi 坐标上的剖面边界换算回物理参数值。
func toPhys(phi []float64, j int, bound float64) float64 {
	if math.IsInf(bound, 0) {
		return bound
	}
	p := append([]float64(nil), phi...)
	p[j] = bound
	return fromPhi(p)[j]
}
