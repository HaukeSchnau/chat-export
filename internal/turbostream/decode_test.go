package turbostream

import (
	"reflect"
	"testing"
)

func TestDecode(t *testing.T) {
	// Encoded from: {a: [1, "x", null, undefined], b: {a: 1}, d: new Date(0), p: Promise}
	// with shared string "a" and the inner object deduplicated by index.
	payload := `[{"_1":2,"_5":6,"_7":8,"_9":10},"a",[3,4,-5,-7],1,"x","b",{"_1":3},"d",["D","1970-01-01T00:00:00.000Z"],"p",["P",11]]` + "\n" + `P11:[{}]` + "\n"

	got, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"a": []any{float64(1), "x", nil, nil},
		"b": map[string]any{"a": float64(1)},
		"d": "1970-01-01T00:00:00.000Z",
		"p": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestDecodeCycle(t *testing.T) {
	// An object whose "self" key points back at itself must not recurse forever.
	got, err := Decode(`[{"_1":0},"self"]`)
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	if obj["self"].(map[string]any)["self"] == nil {
		t.Fatal("cycle not preserved")
	}
}
