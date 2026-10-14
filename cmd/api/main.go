package main

import (
	"go.uber.org/fx"
)

func main() {
	app := fx.New(AppModule)
	app.Run()
}
