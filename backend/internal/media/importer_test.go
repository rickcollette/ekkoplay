package media

import "testing"

func TestSafePathComponent(t *testing.T) {
	cases := map[string]string{"AC/DC": "AC_DC", "  ..  ": "Unknown", "A<B>:C": "A_B_C"}
	for in, want := range cases {
		if got := safe(in); got != want {
			t.Errorf("safe(%q)=%q, want %q", in, got, want)
		}
	}
}
func TestNumberPrefix(t *testing.T) {
	for in, want := range map[string]int{"02/12": 2, "3": 3, "": 0} {
		if got := numberPrefix(in); got != want {
			t.Errorf("numberPrefix(%q)=%d", in, got)
		}
	}
}
