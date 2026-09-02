package main

import (
	"flag"
	"log"

	"github.com/SomtoJF/lsm-tree/initializer"
)

func main() {
	db := initializer.InitDB()
	log.Println("database initialized")

	setFlag := flag.Int("set", 0, "To save a key-value pair, use the -set flag followed by the key. For example:	-set key1")
	valueFlag := flag.String("v", "", "To save a key-value pair, use the -v flag followed by the value. For example:	-v value1")
	getFlag := flag.Int("get", 0, "To get a key-value pair, use the -get flag followed by the key. For example:	-get key1")
	flag.Parse()

	if *setFlag != 0 {
		if *valueFlag == "" {
			panic("Error: The -v flag is required when using the -set flag.")
		}
		println(db.Set(*setFlag, *valueFlag))
	} else if *getFlag != 0 {
		println(db.Get(*getFlag))
	}
}
