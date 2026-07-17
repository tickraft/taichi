package i18n

import "fmt"

// sprintf is a thin wrapper around fmt.Sprintf, convenient for replacement in tests.
func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
