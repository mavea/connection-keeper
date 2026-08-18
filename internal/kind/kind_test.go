package kind

import "testing"

func TestKindIDReturnsStoredValue(t *testing.T) {
	k := &kind[any]{id: 7, s: "http"}

	if got := k.ID(); got != 7 {
		t.Fatalf("ID() = %d, ожидается 7", got)
	}
}

func TestKindStringReturnsStoredValue(t *testing.T) {
	k := &kind[any]{id: 7, s: "http"}

	if got := k.String(); got != "http" {
		t.Fatalf("String() = %q, ожидается %q", got, "http")
	}
}

func TestKindZeroValue(t *testing.T) {
	var k kind[any]

	if got := k.ID(); got != 0 {
		t.Fatalf("ID() для нулевого значения = %d, ожидается 0", got)
	}
	if got := k.String(); got != "" {
		t.Fatalf("String() для нулевого значения = %q, ожидается пустая строка", got)
	}
}
