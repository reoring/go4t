module example.com/sql-service

go 1.25.0

require (
	gofx v0.0.0
	github.com/prometheus/client_golang v1.24.1
	modernc.org/sqlite v1.59.0
)

replace gofx => ../..
