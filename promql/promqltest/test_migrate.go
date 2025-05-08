package promqltest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/regexp"
)

type MigrateMode int

const (
	MigrateStrict MigrateMode = iota
	MigrateBasic
	MigrateTolerant
)

func ParseMigrateMode(s string) (MigrateMode, error) {
	switch s {
	case "strict":
		return MigrateStrict, nil
	case "basic":
		return MigrateBasic, nil
	case "tolerant":
		return MigrateTolerant, nil
	default:
		return MigrateStrict, fmt.Errorf("invalid mode: %s", s)
	}
}

// MigrateTestData migrates all PromQL test files to the new syntax format.
// It applies annotation rules based on the provided migration mode ("strict", "basic", or "tolerant").
// The function parses each .test file, converts it to the new syntax and overwrites the file.
func MigrateTestData(mode string) error {
	const dir = "promql/promqltest/testdata"
	migrationMode, err := ParseMigrateMode(mode)
	if err != nil {
		return fmt.Errorf("failed to parse mode: %w", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read testdata directory: %w", err)
	}

	annotationMap := map[MigrateMode]map[string][]string{
		MigrateStrict: {
			"eval_fail":    {"expect fail", "expect no_warn", "expect no_info"},
			"eval_warn":    {"expect warn", "expect no_info"},
			"eval_info":    {"expect info", "expect no_warn"},
			"eval_ordered": {"expect ordered", "expect no_warn", "expect no_info"},
			"eval":         {"expect no_warn", "expect no_info"},
		},
		MigrateBasic: {
			"eval_fail":    {"expect fail"},
			"eval_warn":    {"expect warn"},
			"eval_info":    {"expect info"},
			"eval_ordered": {"expect ordered"},
		},
		MigrateTolerant: {
			"eval_fail":    {"expect fail"},
			"eval_ordered": {"expect ordered"},
		},
	}

	evalRegex := regexp.MustCompile(`^(eval |eval_fail |eval_warn |eval_info |eval_ordered )(.*)$`)

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".test") {
			continue
		}

		path := filepath.Join(dir, file.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		lines := strings.Split(string(content), "\n")
		processedLines, err := processTestFileLines(lines, annotationMap[migrationMode], evalRegex)
		if err != nil {
			return fmt.Errorf("error processing file %s: %w", path, err)
		}

		if err := os.WriteFile(path, []byte(strings.Join(processedLines, "\n")), 0o644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}
	return nil
}

func processTestFileLines(
	lines []string,
	annotationMap map[string][]string,
	evalRegex *regexp.Regexp,
) (result []string, err error) {
	var inputBlock []string
	var outputBlock []string
	for i := 0; i < len(lines); i += len(inputBlock) {
		inputBlock = nil
		outputBlock = nil
		matches := evalRegex.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if matches == nil {
			inputBlock = append(inputBlock, lines[i])
			result = append(result, lines[i])
			continue
		}

		skipBlock := false
		for j := i + 1; j < len(lines) && !evalRegex.MatchString(strings.TrimSpace(lines[j])); j++ {
			inputBlock = append(inputBlock, lines[j])
			if strings.Contains(lines[j], "expect ") {
				skipBlock = true
			}
		}

		if skipBlock {
			result = append(result, lines[i])
			i++
			result = append(result, inputBlock...)
			continue
		}

		// Detecting indentation style (tab or space) from the first non-empty, indented line
		indent := "  "
		for _, line := range inputBlock {
			trimmed := strings.TrimLeft(line, " \t")
			if len(trimmed) < len(line) {
				indent = line[:len(line)-len(trimmed)]
				break
			}
		}

		command := strings.TrimSpace(matches[1])
		expression := matches[2]
		var annotations []string
		result = append(result, fmt.Sprintf("eval %s", expression))
		i++

		for _, annotation := range annotationMap[command] {
			annotations = append(annotations, indent+annotation)
		}

		for _, line := range inputBlock {
			trimmedLine := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmedLine, "expected_fail_message"):
				msg := strings.TrimPrefix(trimmedLine, "expected_fail_message ")
				for j, s := range annotations {
					if strings.Contains(s, "expect fail") {
						annotations[j] = indent + fmt.Sprintf("expect fail msg:%s", msg)
					}
				}
			case strings.HasPrefix(trimmedLine, "expected_fail_regexp"):
				regex := strings.TrimPrefix(trimmedLine, "expected_fail_regexp ")
				for j, s := range annotations {
					if strings.Contains(s, "expect fail") {
						annotations[j] = indent + fmt.Sprintf("expect fail regex:%s", regex)
					}
				}
			default:
				outputBlock = append(outputBlock, line)
			}
		}

		result = append(result, annotations...)
		result = append(result, outputBlock...)
	}

	return result, nil
}
