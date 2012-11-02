package ranges

import "testing"
import "fmt"

func TestSplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{ "kn1", "kn2", "kn3", "kn100" }
	input := "kn[1-3,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}