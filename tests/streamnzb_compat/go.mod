module dracula-streamnzb-compat

go 1.26.0

require (
	github.com/dreulavelle/jhin v0.8.0
	streamnzb v0.0.0
)

require golang.org/x/text v0.42.0 // indirect

replace streamnzb => ../../.streamnzb-compat
