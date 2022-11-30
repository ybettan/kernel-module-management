package utils

import "fmt"

func WarnString(str string) string {
	// return an eerror
	return fmt.Sprintf("WARNING: %s", str)
}
