module github.com/ArchmageTony/cpa-resin-sticky-bind

go 1.26.10

require (
	github.com/router-for-me/CLIProxyAPI/v7 v7.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/router-for-me/CLIProxyAPI/v7 => ./tmp/CLIProxyAPI-src
