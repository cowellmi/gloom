//go:build feather_m0 && no_hypnos

package main

func initWing() Profile {
	return Profile{
		Rails: fallback.Rails,
	}
}
