package utils

import "fmt"

func WarnString(str string) string {
	// comment
	return fmt.Sprintf("WARNING: %s", str)
}
