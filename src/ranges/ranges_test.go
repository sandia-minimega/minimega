package ranges

import "testing"
import "fmt"
//BUG(floren): this is not a good test
func TestSplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	res, _ := r.SplitRange("kn[1-20,5,10]")
	fmt.Println(res)
}
