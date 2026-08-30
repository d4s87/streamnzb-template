package stremio

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

const draculaFormatterPrefix = "SNZBF1:"

type draculaFormatterPayload struct {
	StreamNZBFormatProfile    int    `json:"streamnzb_format_profile"`
	Name                      string `json:"name"`
	ResultNameTemplate        string `json:"result_name_template"`
	ResultDescriptionTemplate string `json:"result_description_template"`
}

type draculaFormatterFixtureFile struct {
	Cases []draculaFormatterCase `json:"cases"`
}

type draculaFormatterCase struct {
	Name         string          `json:"name"`
	Context      json.RawMessage `json:"context"`
	Contains     []string        `json:"contains,omitempty"`
	Excludes     []string        `json:"excludes,omitempty"`
	Counts       map[string]int  `json:"counts,omitempty"`
	NameContains []string        `json:"name_contains,omitempty"`
	NameExcludes []string        `json:"name_excludes,omitempty"`
	NameCounts   map[string]int  `json:"name_counts,omitempty"`
}

func decodeDraculaFormatterShareCode(raw []byte) ([]byte, error) {
	code := strings.TrimSpace(string(raw))

	if !strings.HasPrefix(code, draculaFormatterPrefix) {
		return nil, fmt.Errorf(
			"formatter does not start with %q",
			draculaFormatterPrefix,
		)
	}

	encoded := strings.TrimPrefix(
		code,
		draculaFormatterPrefix,
	)

	compressed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf(
			"decode formatter Base64URL: %w",
			err,
		)
	}

	reader, err := gzip.NewReader(
		bytes.NewReader(compressed),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"open formatter gzip payload: %w",
			err,
		)
	}
	defer reader.Close()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf(
			"read formatter gzip payload: %w",
			err,
		)
	}

	return decoded, nil
}

func loadDraculaFormatter(
	t *testing.T,
) draculaFormatterPayload {
	t.Helper()

	path := os.Getenv("DRACULA_FORMATTER_PATH")
	if path == "" {
		t.Fatal("DRACULA_FORMATTER_PATH is not set")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read formatter %q: %v",
			path,
			err,
		)
	}

	raw := data

	if strings.HasPrefix(
		strings.TrimSpace(string(data)),
		draculaFormatterPrefix,
	) {
		raw, err = decodeDraculaFormatterShareCode(data)
		if err != nil {
			t.Fatal(err)
		}
	}

	var payload draculaFormatterPayload

	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf(
			"decode formatter payload %q: %v",
			path,
			err,
		)
	}

	if payload.StreamNZBFormatProfile != 1 {
		t.Fatalf(
			"unsupported formatter schema: got %d, expected 1",
			payload.StreamNZBFormatProfile,
		)
	}

	if strings.TrimSpace(
		payload.ResultDescriptionTemplate,
	) == "" {
		t.Fatal(
			"result_description_template is empty",
		)
	}

	if err := ValidateResultTemplates(
		payload.ResultNameTemplate,
		payload.ResultDescriptionTemplate,
	); err != nil {
		t.Fatalf(
			"StreamNZB rejected formatter templates: %v",
			err,
		)
	}

	return payload
}

func loadDraculaFormatterFixtures(
	t *testing.T,
) draculaFormatterFixtureFile {
	t.Helper()

	path := os.Getenv(
		"DRACULA_FORMATTER_CASES_PATH",
	)

	if path == "" {
		t.Fatal(
			"DRACULA_FORMATTER_CASES_PATH is not set",
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read formatter fixtures %q: %v",
			path,
			err,
		)
	}

	var fixtures draculaFormatterFixtureFile

	if err := json.Unmarshal(
		data,
		&fixtures,
	); err != nil {
		t.Fatalf(
			"decode formatter fixtures: %v",
			err,
		)
	}

	if len(fixtures.Cases) == 0 {
		t.Fatal(
			"formatter fixture file contains no cases",
		)
	}

	return fixtures
}

func renderDraculaFormatter(
	t *testing.T,
	templateText string,
	ctx FormatContext,
) string {
	t.Helper()

	tpl, err := compileFormatTemplate(
		templateText,
	)

	if err != nil {
		t.Fatalf(
			"compile formatter through StreamNZB: %v",
			err,
		)
	}

	const fallback = "DRACULA_FORMATTER_FALLBACK"

	rendered := renderResultTemplate(
		tpl,
		ctx,
		fallback,
	)

	if rendered == fallback {
		t.Fatal(
			"formatter unexpectedly used StreamNZB fallback rendering",
		)
	}

	return rendered
}

func TestDraculaFormatterFixtures(
	t *testing.T,
) {
	payload := loadDraculaFormatter(t)
	fixtures := loadDraculaFormatterFixtures(t)

	for _, fixture := range fixtures.Cases {
		fixture := fixture

		t.Run(
			fixture.Name,
			func(t *testing.T) {
				var ctx FormatContext

				if len(fixture.Context) > 0 {
					if err := json.Unmarshal(
						fixture.Context,
						&ctx,
					); err != nil {
						t.Fatalf(
							"decode FormatContext: %v",
							err,
						)
					}
				}

				// Most small regression fixtures only need the description.
				// Rich end-to-end fixtures opt into result-name rendering with
				// name_contains/name_excludes/name_counts. This avoids forcing
				// sparse contexts through a name template that legitimately
				// renders empty and therefore triggers StreamNZB's fallback.
				if len(fixture.NameContains) > 0 ||
					len(fixture.NameExcludes) > 0 ||
					len(fixture.NameCounts) > 0 {
					renderedName := renderDraculaFormatter(
						t,
						payload.ResultNameTemplate,
						ctx,
					)

					t.Logf(
						"rendered result name:\n%s",
						renderedName,
					)

					for _, expected := range fixture.NameContains {
						if !strings.Contains(
							renderedName,
							expected,
						) {
							t.Errorf(
								"expected result name to contain %q\nrendered:\n%s",
								expected,
								renderedName,
							)
						}
					}

					for _, unexpected := range fixture.NameExcludes {
						if strings.Contains(
							renderedName,
							unexpected,
						) {
							t.Errorf(
								"expected result name not to contain %q\nrendered:\n%s",
								unexpected,
								renderedName,
							)
						}
					}

					for text, expectedCount := range fixture.NameCounts {
						actual := strings.Count(
							renderedName,
							text,
						)

						if actual != expectedCount {
							t.Errorf(
								"%q rendered %d times in result name, want %d\nrendered:\n%s",
								text,
								actual,
								expectedCount,
								renderedName,
							)
						}
					}
				}

				rendered := renderDraculaFormatter(
					t,
					payload.ResultDescriptionTemplate,
					ctx,
				)

				// Deliberately visible with go test -v.
				// This is the formatter simulation output.
				t.Logf(
					"rendered result description:\n%s",
					rendered,
				)

				for _, expected := range fixture.Contains {
					if !strings.Contains(
						rendered,
						expected,
					) {
						t.Errorf(
							"expected output to contain %q\nrendered:\n%s",
							expected,
							rendered,
						)
					}
				}

				for _, unexpected := range fixture.Excludes {
					if strings.Contains(
						rendered,
						unexpected,
					) {
						t.Errorf(
							"expected output not to contain %q\nrendered:\n%s",
							unexpected,
							rendered,
						)
					}
				}

				for text, expectedCount := range fixture.Counts {
					actual := strings.Count(
						rendered,
						text,
					)

					if actual != expectedCount {
						t.Errorf(
							"%q rendered %d times, want %d\nrendered:\n%s",
							text,
							actual,
							expectedCount,
							rendered,
						)
					}
				}
			},
		)
	}
}
