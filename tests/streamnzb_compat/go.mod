module dracula-streamnzb-compat

go 1.25.6

require (
	github.com/dreulavelle/jhin v0.6.0
	streamnzb v0.0.0
)

require golang.org/x/text v0.41.0 // indirect

replace streamnzb => ../../.streamnzb-compat
