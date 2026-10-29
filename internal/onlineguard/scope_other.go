//go:build !windows

package onlineguard

func unrelatedLocalUserBlock(RuleSpec, []Backend) bool { return false }
