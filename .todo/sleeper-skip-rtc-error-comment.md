# Resolve `// TODO: short explanation of skip err` in sleeper.go

**severity:** low

`sleeper.Sleep` discards the error from `ReadTime` before calculating `remaining` (sleeper.go:103). The skip is intentional: a failed read leaves `now` as the zero time, making `remaining` enormous, which still routes correctly into deep sleep with the original absolute `target`. However, if `now` returns a slightly-future bogus time, `remaining` goes negative, `shouldSleep` becomes false, and `Sleep` returns immediately — causing the manager loop to spin without sleeping. Replace the TODO comment with an explanation of this invariant, or handle the negative-remaining case explicitly.
