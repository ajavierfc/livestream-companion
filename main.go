package main

import (
	"livestream-companion/management"
	"livestream-companion/routes"
)

func main() {
	management.InitializeDatabase()

	r := routes.SetupRouter()
	r.Run(":5004")
}
