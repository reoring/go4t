module example.com/app

go 1.22

require gofx v0.0.0
require example.com/lib v1.0.0

replace gofx => ../../..
replace example.com/lib => ../lib
