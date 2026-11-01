module github.com/reoring/gofx/ports/client-go

go 1.26.0

require (
	gofx v0.0.0
	k8s.io/api v0.37.2
	k8s.io/apimachinery v0.37.2
)

replace gofx => ../..
