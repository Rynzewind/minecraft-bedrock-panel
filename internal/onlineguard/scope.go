package onlineguard

import "regexp"

// Only one ordinary domain-account SID grant is understood; unknown scopes
// remain blocking. No rule names or product-specific exceptions are used.
var simpleLocalUser = regexp.MustCompile(`^(?:O:LS)?D:\(A;;CC;;;(S-1-5-21-[0-9]+-[0-9]+-[0-9]+-[0-9]+)\)$`)

func localUserSID(values []string) string {
	if len(values) != 1 {
		return ""
	}
	m := simpleLocalUser.FindStringSubmatch(values[0])
	if m == nil {
		return ""
	}
	return m[1]
}
