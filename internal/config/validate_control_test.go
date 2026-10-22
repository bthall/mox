package config

import (
	"fmt"
	"testing"
)

func TestRejectTerminalControlCharacters(t *testing.T) {
	for char := rune(0); char <= 0x9f; char++ {
		if char >= 0x20 && char < 0x7f {
			continue
		}
		t.Run(fmt.Sprintf("U+%04X", char), func(t *testing.T) {
			command := "echo harmless" + string(char) + "echo unintended"
			if err := validateNoControlChars([]string{"echo safe", command}, "commands"); err == nil {
				t.Fatal("command accepted terminal control character")
			}
			if err := validateStringNoControlChars("ssh "+string(char)+"{{host}}", "connect"); err == nil {
				t.Fatal("connect template accepted terminal control character")
			}
		})
	}
}

func TestAllowPrintableCommandCharacters(t *testing.T) {
	for _, command := range []string{"", "echo 'hello world'", "printf '%s' café", "ssh {{host}}"} {
		if err := validateNoControlChars([]string{command}, "commands"); err != nil {
			t.Fatal(err)
		}
		if err := validateStringNoControlChars(command, "connect"); err != nil {
			t.Fatal(err)
		}
	}
}
