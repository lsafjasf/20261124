package tagfix

import "fmt"

// removeEmulationPrevention returns the RBSP together with a mapping from each
// RBSP byte to its byte index in the original EBSP.
func removeEmulationPrevention(ebsp []byte) ([]byte, []int) {
	rbsp := make([]byte, 0, len(ebsp))
	mapping := make([]int, 0, len(ebsp))
	for i, value := range ebsp {
		if i >= 2 && value == 0x03 && ebsp[i-1] == 0 && ebsp[i-2] == 0 {
			continue
		}
		rbsp = append(rbsp, value)
		mapping = append(mapping, i)
	}
	return rbsp, mapping
}

func insertEmulationPrevention(rbsp []byte) []byte {
	ebsp := make([]byte, 0, len(rbsp)+len(rbsp)/64+1)
	for i, value := range rbsp {
		if i >= 2 && rbsp[i-1] == 0 && rbsp[i-2] == 0 && value <= 0x03 {
			ebsp = append(ebsp, 0x03)
		}
		ebsp = append(ebsp, value)
	}
	return ebsp
}

func checkNALHeader(nal []byte, want byte) error {
	if len(nal) < 2 {
		return fmt.Errorf("%w: NAL unit is too short", ErrUnsupportedMedia)
	}
	if nal[0]&0x1f != want {
		return fmt.Errorf("%w: NAL type is %d, want %d",
			ErrUnsupportedMedia, nal[0]&0x1f, want)
	}
	return nil
}
