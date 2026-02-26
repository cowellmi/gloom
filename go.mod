module github.com/cowellmi/gloom

go 1.24.0

require (
	github.com/andreyvit/tinyjson v0.0.2
	github.com/blues/note-tinygo v0.0.0-20211128142428-6d595f1248a7
	tinygo.org/x/drivers v0.34.1-0.20260212120525-d6114c9f6dee
	tinygo.org/x/tinyfs v0.5.0
)

require (
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/valyala/fastjson v1.6.3 // indirect
)

replace tinygo.org/x/tinyfs => github.com/cowellmi/tinyfs v0.5.1-0.20260215224512-3de04b468176

replace tinygo.org/x/drivers => github.com/cowellmi/drivers v0.34.1-0.20260222013331-70fcaddea195
