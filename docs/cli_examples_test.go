package docs_test

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/flidai/leapview/docs"
	"github.com/flidai/leapview/internal/app/cli"
)

// TestDocumentedLeapViewCommandsUseRealCommandsAndFlags keeps shell examples in
// the authored guides honest: every fenced `leapview ...` command must name a
// real command path and only flags that command (or an ancestor) defines. The
// inventory is the live Cobra tree that also generates the CLI reference, so a
// renamed or removed flag fails here instead of in a reader's terminal.
func TestDocumentedLeapViewCommandsUseRealCommandsAndFlags(t *testing.T) {
	root := cli.NewCommand(context.Background())

	checked := 0
	err := fs.WalkDir(docs.Files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Generated references are rendered from the same tree and are covered
		// by their own snapshot checks.
		if entry.IsDir() || path.Ext(name) != ".md" || strings.HasPrefix(name, "reference/") || strings.HasPrefix(name, "api/") {
			return nil
		}
		contents, err := docs.Files.ReadFile(name)
		if err != nil {
			return err
		}
		for _, example := range leapviewShellCommands(string(contents)) {
			checked++
			if problem := checkLeapViewCommand(root, example.words); problem != "" {
				t.Errorf("docs/%s:%d: %s\n\t%s", name, example.line, problem, strings.Join(example.words, " "))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("checked only %d documented leapview commands; the fenced-command extractor is likely broken", checked)
	}
}

func TestLeapViewCommandCheckerRejectsUnknownCommandsAndFlags(t *testing.T) {
	root := &cobra.Command{Use: "leapview"}
	root.PersistentFlags().String("target", "", "")
	group := &cobra.Command{Use: "dashboards"}
	list := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Run: func(*cobra.Command, []string) {}}
	list.Flags().String("project-id", "", "")
	list.Flags().BoolP("json", "j", false, "")
	group.AddCommand(list)
	root.AddCommand(group)

	for _, test := range []struct {
		command string
		wantErr string
	}{
		{"leapview dashboards list --project-id p --target t", ""},
		{"leapview --target t dashboards ls -j --project-id=p", ""},
		{"leapview dashboards list --help", ""},
		{"leapview dashboards list --project project:commerce", `unknown flag --project`},
		{"leapview dashboards list -x", `unknown shorthand flag -x`},
		{"leapview dashboards lst", `unknown command "lst"`},
		{"leapview nope list", `unknown command "nope"`},
	} {
		blocks := leapviewShellCommands("```sh\n" + test.command + "\n```\n")
		if len(blocks) != 1 {
			t.Fatalf("%q extracted %d commands, want 1", test.command, len(blocks))
		}
		got := checkLeapViewCommand(root, blocks[0].words)
		switch {
		case test.wantErr == "" && got != "":
			t.Errorf("%q rejected: %s", test.command, got)
		case test.wantErr != "" && !strings.Contains(got, test.wantErr):
			t.Errorf("%q problem = %q, want containing %q", test.command, got, test.wantErr)
		}
	}
}

func TestLeapViewShellCommandExtraction(t *testing.T) {
	markdown := "Prose: leapview not-a-command --nope\n\n" +
		"```sh\n" +
		"# comment leapview ignored --nope\n" +
		"$ LEAPVIEW_TOKEN=x leapview a b \\\n  --one '{\"k\": \"two words\"}' | jq .\n" +
		"echo hi && leapview c --two > out.json\n" +
		"```\n\n" +
		"```yaml\nleapview: ignored --nope\n```\n\n" +
		"```text\nleapview output --nope\n```\n"
	got := leapviewShellCommands(markdown)
	want := [][]string{
		{"leapview", "a", "b", "--one", `{"k": "two words"}`},
		{"leapview", "c", "--two"},
	}
	if len(got) != len(want) {
		t.Fatalf("extracted %d commands %v, want %d", len(got), got, len(want))
	}
	for index := range want {
		if fmt.Sprint(got[index].words) != fmt.Sprint(want[index]) {
			t.Errorf("command %d = %q, want %q", index, got[index].words, want[index])
		}
	}
}

type shellExample struct {
	line  int
	words []string
}

var (
	shellFenceOpen    = regexp.MustCompile("^\\s*(?:```|~~~)\\s*([A-Za-z0-9_+-]*)")
	shellFenceClose   = regexp.MustCompile("^\\s*(?:```|~~~)\\s*$")
	shellEnvAssigment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	shellLanguages    = map[string]bool{"sh": true, "bash": true, "shell": true, "zsh": true, "console": true}
)

// leapviewShellCommands returns every shell command in fenced shell blocks that
// invokes the leapview binary, with line continuations, pipelines, command
// lists, redirections, comments, and leading environment assignments resolved.
func leapviewShellCommands(markdown string) []shellExample {
	var examples []shellExample
	lines := strings.Split(markdown, "\n")
	for index := 0; index < len(lines); index++ {
		open := shellFenceOpen.FindStringSubmatch(lines[index])
		if open == nil {
			continue
		}
		shell := shellLanguages[strings.ToLower(open[1])]
		index++
		for ; index < len(lines) && !shellFenceClose.MatchString(lines[index]); index++ {
			if !shell {
				continue
			}
			start := index
			logical := strings.TrimRight(lines[index], " \t")
			for strings.HasSuffix(logical, "\\") && index+1 < len(lines) && !shellFenceClose.MatchString(lines[index+1]) {
				index++
				logical = strings.TrimSuffix(logical, "\\") + " " + strings.TrimSpace(lines[index])
			}
			for _, words := range shellSimpleCommands(logical) {
				for len(words) > 0 && (words[0] == "$" || shellEnvAssigment.MatchString(words[0])) {
					words = words[1:]
				}
				if len(words) > 0 && words[0] == "leapview" {
					examples = append(examples, shellExample{line: start + 1, words: words})
				}
			}
		}
	}
	return examples
}

// shellSimpleCommands tokenizes one logical shell line, honouring single and
// double quotes, and splits it at unquoted control operators. A redirection
// ends the argument list of its command.
func shellSimpleCommands(line string) [][]string {
	var (
		commands [][]string
		words    []string
		word     strings.Builder
		inWord   bool
		quote    rune
		skipping bool
	)
	endWord := func() {
		if inWord && !skipping {
			words = append(words, word.String())
		}
		word.Reset()
		inWord = false
	}
	endCommand := func() {
		endWord()
		if len(words) > 0 {
			commands = append(commands, words)
		}
		words = nil
		skipping = false
	}
	runes := []rune(line)
	for position := 0; position < len(runes); position++ {
		character := runes[position]
		switch {
		case quote != 0:
			if character == quote {
				quote = 0
			} else if character == '\\' && quote == '"' && position+1 < len(runes) {
				position++
				word.WriteRune(runes[position])
			} else {
				word.WriteRune(character)
			}
		case character == '\'' || character == '"':
			quote, inWord = character, true
		case character == '\\' && position+1 < len(runes):
			position++
			word.WriteRune(runes[position])
			inWord = true
		case character == '#' && !inWord:
			endCommand()
			return commands
		case character == ' ' || character == '\t':
			endWord()
		case character == '|' || character == ';' || character == '&':
			endCommand()
		case character == '>' || character == '<':
			endWord()
			skipping = true
		default:
			word.WriteRune(character)
			inWord = true
		}
	}
	endCommand()
	return commands
}

// checkLeapViewCommand reports the first problem with a documented invocation:
// an unknown command path or a flag the resolved command does not accept.
func checkLeapViewCommand(root *cobra.Command, words []string) string {
	arguments := words[1:]
	command := root
	descending := true
	for position := 0; position < len(arguments); position++ {
		word := arguments[position]
		if strings.HasPrefix(word, "-") && word != "-" {
			// A separated flag value is not a command word.
			if name, hasValue := longFlagName(word); name != "" && !hasValue {
				if flag := lookupFlag(command, name); flag != nil && flag.NoOptDefVal == "" {
					position++
				}
			}
			continue
		}
		if !descending {
			continue
		}
		if command == root && (word == "help" || word == "completion") {
			return ""
		}
		if child := findChild(command, word); child != nil {
			command = child
			continue
		}
		if command.HasSubCommands() && !command.Runnable() {
			return fmt.Sprintf("unknown command %q for %q", word, command.CommandPath())
		}
		// Remaining words are positional arguments of a runnable command.
		descending = false
	}

	for _, word := range arguments {
		switch {
		case word == "--":
			return ""
		case strings.HasPrefix(word, "--"):
			name, _ := longFlagName(word)
			if name == "help" || lookupFlag(command, name) != nil {
				continue
			}
			return fmt.Sprintf("unknown flag --%s for %q (accepts: %s)", name, command.CommandPath(), flagInventory(command))
		case strings.HasPrefix(word, "-") && len(word) > 1 && !isNumber(word):
			shorthand := string([]rune(word)[1])
			if shorthand == "h" || lookupShorthand(command, shorthand) != nil {
				continue
			}
			return fmt.Sprintf("unknown shorthand flag -%s for %q", shorthand, command.CommandPath())
		}
	}
	return ""
}

// longFlagName returns the name in --name or --name=value and whether the
// value was attached.
func longFlagName(word string) (name string, hasValue bool) {
	if !strings.HasPrefix(word, "--") {
		return "", false
	}
	name, _, hasValue = strings.Cut(strings.TrimPrefix(word, "--"), "=")
	return name, hasValue
}

func findChild(command *cobra.Command, name string) *cobra.Command {
	for _, child := range command.Commands() {
		if child.Name() == name || child.HasAlias(name) {
			return child
		}
	}
	return nil
}

func lookupFlag(command *cobra.Command, name string) *pflag.Flag {
	if flag := command.LocalFlags().Lookup(name); flag != nil {
		return flag
	}
	return command.InheritedFlags().Lookup(name)
}

func lookupShorthand(command *cobra.Command, shorthand string) *pflag.Flag {
	if flag := command.LocalFlags().ShorthandLookup(shorthand); flag != nil {
		return flag
	}
	return command.InheritedFlags().ShorthandLookup(shorthand)
}

func flagInventory(command *cobra.Command) string {
	var names []string
	add := func(flag *pflag.Flag) { names = append(names, "--"+flag.Name) }
	command.LocalFlags().VisitAll(add)
	command.InheritedFlags().VisitAll(add)
	return strings.Join(names, " ")
}

func isNumber(word string) bool {
	for _, character := range strings.TrimPrefix(word, "-") {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
