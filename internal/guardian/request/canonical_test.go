package request

import "testing"

func TestCanonicalCmd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "empty",
			args: []string{},
			want: "",
		},
		{
			name: "single_simple",
			args: []string{"ls"},
			want: "ls",
		},
		{
			name: "simple_command",
			args: []string{"docker", "ps"},
			want: "docker ps",
		},
		{
			name: "command_with_flags",
			args: []string{"ls", "-la", "/tmp"},
			want: "ls -la /tmp",
		},
		{
			name: "docker_compose",
			args: []string{"docker", "compose", "up", "-d"},
			want: "docker compose up -d",
		},
		{
			name: "arg_with_space",
			args: []string{"echo", "hello world"},
			want: "echo 'hello world'",
		},
		{
			name: "arg_with_single_quote",
			args: []string{"echo", "it's"},
			want: "echo 'it'\\''s'",
		},
		{
			name: "arg_with_double_quotes",
			args: []string{"echo", "say \"hello\""},
			want: "echo 'say \"hello\"'",
		},
		{
			name: "arg_with_special_chars",
			args: []string{"echo", "foo;bar"},
			want: "echo 'foo;bar'",
		},
		{
			name: "arg_with_dollar",
			args: []string{"echo", "$HOME"},
			want: "echo '$HOME'",
		},
		{
			name: "arg_with_backtick",
			args: []string{"echo", "`whoami`"},
			want: "echo '`whoami`'",
		},
		{
			name: "arg_with_newline",
			args: []string{"echo", "line1\nline2"},
			want: "echo 'line1\nline2'",
		},
		{
			name: "empty_arg",
			args: []string{"cmd", ""},
			want: "cmd ''",
		},
		{
			name: "multiple_quotes",
			args: []string{"echo", "it's a 'test'"},
			want: "echo 'it'\\''s a '\\''test'\\'''",
		},
		{
			name: "path_with_colon",
			args: []string{"echo", "PATH=/usr/bin:/bin"},
			want: "echo PATH=/usr/bin:/bin",
		},
		{
			name: "url_with_at",
			args: []string{"git", "clone", "git@github.com:user/repo.git"},
			want: "git clone git@github.com:user/repo.git",
		},
		{
			name: "version_with_plus",
			args: []string{"go", "get", "example.com/pkg@v1.0.0+incompatible"},
			want: "go get example.com/pkg@v1.0.0+incompatible",
		},
		{
			name: "asterisk_needs_quoting",
			args: []string{"ls", "*.go"},
			want: "ls '*.go'",
		},
		{
			name: "pipe_needs_quoting",
			args: []string{"echo", "foo|bar"},
			want: "echo 'foo|bar'",
		},
		{
			name: "ampersand_needs_quoting",
			args: []string{"echo", "foo&bar"},
			want: "echo 'foo&bar'",
		},
		{
			name: "parentheses_need_quoting",
			args: []string{"echo", "(test)"},
			want: "echo '(test)'",
		},
		{
			name: "brackets_need_quoting",
			args: []string{"echo", "[test]"},
			want: "echo '[test]'",
		},
		{
			name: "braces_need_quoting",
			args: []string{"echo", "{a,b}"},
			want: "echo '{a,b}'",
		},
		{
			name: "less_than_needs_quoting",
			args: []string{"echo", "a<b"},
			want: "echo 'a<b'",
		},
		{
			name: "greater_than_needs_quoting",
			args: []string{"echo", "a>b"},
			want: "echo 'a>b'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalCmd(tt.args)
			if got != tt.want {
				t.Errorf("canonicalCmd(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "''"},
		{"simple", "hello", "hello"},
		{"with_hyphen", "-la", "-la"},
		{"with_underscore", "foo_bar", "foo_bar"},
		{"with_dot", "file.txt", "file.txt"},
		{"with_slash", "/usr/bin", "/usr/bin"},
		{"with_colon", "host:port", "host:port"},
		{"with_at", "user@host", "user@host"},
		{"with_plus", "v1.0+beta", "v1.0+beta"},
		{"with_equals", "KEY=value", "KEY=value"},
		{"with_space", "hello world", "'hello world'"},
		{"with_single_quote", "it's", "'it'\\''s'"},
		{"with_tab", "a\tb", "'a\tb'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSafeChar(t *testing.T) {
	safe := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:@+="
	for _, c := range safe {
		if !isSafeChar(c) {
			t.Errorf("isSafeChar(%q) = false, want true", c)
		}
	}

	unsafe := " \t\n'\"`;$()[]{}|&<>*?!#~\\^"
	for _, c := range unsafe {
		if isSafeChar(c) {
			t.Errorf("isSafeChar(%q) = true, want false", c)
		}
	}
}

func TestContainsNUL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"simple", "hello", false},
		{"with_space", "hello world", false},
		{"nul_at_start", "\x00hello", true},
		{"nul_at_end", "hello\x00", true},
		{"nul_in_middle", "hel\x00lo", true},
		{"only_nul", "\x00", true},
		{"multiple_nul", "\x00\x00", true},
		{"nul_with_special", "foo\x00bar$baz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsNUL(tt.input)
			if got != tt.want {
				t.Errorf("containsNUL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalCmd_AllEmptyArgs(t *testing.T) {
	// Edge case: all empty arguments
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single_empty",
			args: []string{""},
			want: "''",
		},
		{
			name: "two_empty",
			args: []string{"", ""},
			want: "'' ''",
		},
		{
			name: "empty_between_args",
			args: []string{"echo", "", "world"},
			want: "echo '' world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalCmd(tt.args)
			if got != tt.want {
				t.Errorf("canonicalCmd(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
