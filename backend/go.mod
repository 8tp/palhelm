module github.com/palhelm/palhelm

go 1.26.5

require (
	github.com/go-chi/chi/v5 v5.2.4
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/new-world-tools/go-oodle v0.3.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.39.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/sys v0.36.0 // indirect
	modernc.org/libc v1.66.10 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// v0.3.0 imports C solely for C.GoString, making its advertised purego Linux
// path unbuildable with CGO disabled. This API-compatible copy removes that
// unused dependency.
replace github.com/new-world-tools/go-oodle => ./third_party/go-oodle
