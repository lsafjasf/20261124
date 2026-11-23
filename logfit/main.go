package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

var paramNames = [5]string{"A(log斜率)", "B(平移)", "C(电平)", "t(截点)", "S(趾部斜率)"}

func main() {
	modelName := flag.String("model", "slog3", "曲线风格: slog3 | vlog | logc")
	dataFile := flag.String("data", "", "CSV 文件，每行: code,linear（可用 -demo 代替）")
	demo := flag.Bool("demo", false, "用该风格的官方曲线生成带噪合成数据做自检")
	nDemo := flag.Int("n", 12, "demo 模式采样点数")
	noise := flag.Float64("noise", 0.004, "demo 模式码值噪声标准差")
	seed := flag.Int64("seed", 1, "demo 模式随机种子")
	flag.Parse()

	m, ok := models[*modelName]
	if !ok {
		fmt.Fprintln(os.Stderr, "未知模型，可选: slog3 | vlog | logc")
		os.Exit(2)
	}

	var data []Sample
	var truth *[5]float64
	if *demo {
		data = genDemo(m, *nDemo, *noise, *seed)
		truth = &m.Canon
	} else {
		var err error
		data, err = readCSV(*dataFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取数据失败:", err)
			os.Exit(1)
		}
	}
	if len(data) < 3 {
		fmt.Fprintln(os.Stderr, "样本太少（<3），无法拟合")
		os.Exit(1)
	}

	r := fit(m, data, nil)
	printReport(m, data, r, truth)
}

// genDemo 在 [1e-3, 4] 线性域对数等距采样，并额外加入黑电平附近的点。
func genDemo(m Model, n int, noise float64, seed int64) []Sample {
	rng := rand.New(rand.NewSource(seed))
	gauss := func() float64 { // Box-Muller
		u1 := 1 - rng.Float64()
		u2 := rng.Float64()
		return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	}
	data := make([]Sample, 0, n)
	nLog := n - n/4 // 3/4 的点对数等距，1/4 集中在趾部附近
	for i := 0; i < nLog; i++ {
		L := math.Exp(math.Log(1e-3) + float64(i)/float64(nLog-1)*math.Log(4/1e-3))
		data = append(data, Sample{L, codeValue(m.Canon, L) + noise*gauss()})
	}
	for i := 0; i < n-nLog; i++ {
		L := 0.002 + 0.012*rng.Float64()
		data = append(data, Sample{L, codeValue(m.Canon, L) + noise*gauss()})
	}
	return data
}

// readCSV 按固定列序 "code,linear"（码值,线性亮度）读取，首行非数字视为表头跳过。
func readCSV(path string) ([]Sample, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data []Sample
	for ln, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == '\t' || r == ';'
		})
		if len(f) < 2 {
			continue
		}
		a, ea := strconv.ParseFloat(strings.TrimSpace(f[0]), 64)
		c, ec := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
		if ea != nil || ec != nil {
			if ln == 0 {
				continue // 表头
			}
			return nil, fmt.Errorf("第 %d 行无法解析: %q", ln+1, line)
		}
		data = append(data, Sample{V: a, L: c})
	}
	return data, nil
}
