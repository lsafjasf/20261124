package j2k

type tagNode struct {
	parent int
	value  int
	low    int
}

type tagLevel struct {
	offset int
	width  int
	height int
}

type tagTree struct {
	width  int
	height int
	levels []tagLevel
	nodes  []tagNode
}

func newTagTree(width, height int) *tagTree {
	if width <= 0 || height <= 0 {
		return nil
	}
	t := &tagTree{width: width, height: height}
	w, h := width, height
	for {
		t.levels = append(t.levels, tagLevel{offset: len(t.nodes), width: w, height: h})
		for i := 0; i < w*h; i++ {
			t.nodes = append(t.nodes, tagNode{parent: -1, value: 999})
		}
		if w*h == 1 {
			break
		}
		w = (w + 1) / 2
		h = (h + 1) / 2
	}
	for i := 0; i+1 < len(t.levels); i++ {
		level := t.levels[i]
		parentLevel := t.levels[i+1]
		for y := 0; y < level.height; y++ {
			for x := 0; x < level.width; x++ {
				node := level.offset + y*level.width + x
				parent := parentLevel.offset +
					(y/2)*parentLevel.width + x/2
				t.nodes[node].parent = parent
			}
		}
	}
	return t
}

func (t *tagTree) reset() {
	for i := range t.nodes {
		t.nodes[i].value = 999
		t.nodes[i].low = 0
	}
}

func (t *tagTree) decode(r *bitReader, leaf, threshold int) (bool, error) {
	if t == nil || leaf < 0 || leaf >= t.width*t.height {
		return false, errTruncatedBits
	}
	path := make([]int, 0, len(t.levels))
	for node := leaf; node >= 0; node = t.nodes[node].parent {
		path = append(path, node)
		if len(path) > len(t.levels) {
			break
		}
	}
	low := 0
	for i := len(path) - 1; i >= 0; i-- {
		node := &t.nodes[path[i]]
		if low > node.low {
			node.low = low
		} else {
			low = node.low
		}
		for low < threshold && low < node.value {
			bit, err := r.readBit()
			if err != nil {
				return false, err
			}
			if bit != 0 {
				node.value = low
			} else {
				low++
			}
		}
		node.low = low
	}
	return t.nodes[leaf].value < threshold, nil
}
