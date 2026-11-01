module example.com/app

go 1.22

require go4t v0.0.0
require example.com/lib v1.0.0

replace go4t => ../../..
replace example.com/lib => ../lib
