module dracula-streamnzb-compat

go 1.25.6

require (
	github.com/dreulavelle/jhin v0.4.1
	streamnzb v0.0.0
)

require (
	github.com/expr-lang/expr v1.17.8 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace streamnzb => ../../.streamnzb-compat
