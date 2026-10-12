package profile

import "testing"

func TestProfileValidation(t *testing.T) {
	got, err := Validate(Input{Name: "  Дастан 🌱  ", Nickname: " TEST_Name ", Avatar: "sprout"})
	if err != nil || got.Name != "Дастан 🌱" || got.Nickname != "test_name" {
		t.Fatal("normalization failed")
	}
	for _, input := range []Input{{Name: "bad\x00name", Avatar: "star"}, {Nickname: "a", Avatar: "star"}, {Avatar: "https://external.test/avatar"}, {Nickname: "друг", Avatar: "star"}} {
		if _, err := Validate(input); err == nil {
			t.Fatal("invalid profile accepted")
		}
	}
}
