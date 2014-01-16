package expect

import "testing"
import "fmt"
import "bytes"

func TestExpect(t *testing.T) {
	input := []byte("Hello\r\nThis is a test\r\nlogin: ")
	e := NewExpecter(bytes.NewReader(input))
	res, err := e.Expect("This ")
	if err != nil {
		t.Fatal("Expect returned: ", res, ", error: ", err)
	}
	res, err = e.Expect("login: ")
	if err != nil {
		t.Fatal("Expect returned: ", res, ", error: ", err)
	}
	fmt.Println("Result: ", res)
}
