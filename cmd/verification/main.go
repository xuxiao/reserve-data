package main

import (
	"fmt"
	"os"

	"github.com/KyberNetwork/reserve-data/cmd/configuration"
)

var noAuthEnable bool
var endpointOW string

func validateArgs() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: ./verification ACTION FLAGS")
	fmt.Println(" deposit -amount AMOUNT -token TOKEN -base_url BASE_URL, base_url default is http://localhost:8000/")
	fmt.Println(" withdraw -amount AMOUNT -token TOKEN -base_url BASE_URL, base_url default is http://localhost:8000/")
}

func run(verify *Verification) {
	switch os.Args[1] {
	default:
		printUsage()
	}
}

func main() {
	var config *configuration.Config
	kyberENV := os.Getenv("KYBER_ENV")
	if kyberENV == "" {
		kyberENV = "dev"
	}
	config = configuration.GetConfig(kyberENV,
		!noAuthEnable,
		endpointOW)
	if config.AuthEngine == nil {
		Warning.Println("Current environment setting does not enable authentication. Please check again!!!")
	}
	verify := NewVerification(config.AuthEngine)
	// verify.RunVerification()
	validateArgs()

	run(verify)
}
