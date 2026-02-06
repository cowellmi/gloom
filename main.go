package main

func main() {
	man, err := NewManager()
	if err != nil {
		println("fatal:", err)
		return
	}

	man.Run()
}
