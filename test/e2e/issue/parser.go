package issue

import (
	"bufio"
	"fmt"
	"strings"
)

type Blocks struct {
	Config      string
	Resources   string
	Expectation string
}

func Parse(body string) (Blocks, error) {
	var blocks Blocks
	var current *string
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			if current != nil {
				*current = strings.TrimSpace(strings.Join(lines, "\n"))
				current = nil
				lines = nil
				continue
			}
			switch strings.TrimSpace(strings.TrimPrefix(line, "```")) {
			case "kwatch-config":
				current = &blocks.Config
			case "kwatch-resources":
				current = &blocks.Resources
			case "kwatch-expectation":
				current = &blocks.Expectation
			}
			continue
		}
		if current != nil {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return Blocks{}, err
	}
	if current != nil {
		return Blocks{}, fmt.Errorf("issue block is not closed")
	}
	if blocks.Config == "" && blocks.Resources == "" {
		return Blocks{}, fmt.Errorf("issue has no kwatch reproduction blocks")
	}
	return blocks, nil
}
