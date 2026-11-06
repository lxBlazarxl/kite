package agent

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	got, err := ReadEnvFile(strings.NewReader("# comment\n\nHUB_URL = http://h:8420\r\nAGENT_TOKEN=a=b\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"HUB_URL": "http://h:8420", "AGENT_TOKEN": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if _, err := ReadEnvFile(strings.NewReader("garbage\n")); err == nil {
		t.Fatal("line without = must be an error")
	}
}

func TestWriteEnvFileRoundTrip(t *testing.T) {
	var b bytes.Buffer
	if err := WriteEnvFile(&b, map[string]string{"B": "2", "A": "1"}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "A=1\nB=2\n" {
		t.Fatalf("got %q", b.String())
	}
	back, err := ReadEnvFile(&b)
	if err != nil || back["A"] != "1" || back["B"] != "2" {
		t.Fatalf("round trip %v %v", back, err)
	}
}
