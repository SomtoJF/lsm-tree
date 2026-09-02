package main

import "flag"

func main() {
	locationFlag := flag.String("l", "lagos", "To get the forecast of a particular location")
	flag.Parse()
	println(*locationFlag)
}
