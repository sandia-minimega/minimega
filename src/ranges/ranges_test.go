package ranges

import "testing"
import "fmt"

func TestSplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	res, _ := r.SplitRange("kn[1-20,5,10]")
	fmt.Println(res)
}
