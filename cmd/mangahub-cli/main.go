package main

func main() {
	if err := Execute(); err != nil {
		fatalf("%v", err)
	}
}
