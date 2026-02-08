package main

func main() {
	man, err := NewManager()
	if err != nil {
		println("fatal:", err.Error())
		return
	}

	man.Run()
}
