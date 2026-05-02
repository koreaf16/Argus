package lane

import "testing"

func TestParseAccountTransition(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want AccountTransition
	}{
		{"empty", "", AccountTransition{Kind: AccountTransitionNone}},
		{"plain ls", "ls -la /tmp", AccountTransition{Kind: AccountTransitionNone}},
		{"psql -c", `psql -c "SELECT now();"`, AccountTransition{Kind: AccountTransitionNone}},

		{"su dash postgres", "su - postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "su"}},
		{"su dash oracle", "su - oracle", AccountTransition{Kind: AccountTransitionEnter, User: "oracle", Method: "su"}},
		{"su login flag", "su -l postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "su"}},
		{"su long login", "su --login postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "su"}},
		{"su no user", "su -", AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "su"}},
		{"su bare", "su", AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "su"}},
		{"su user only", "su postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "su"}},
		{"su -c inline", `su - postgres -c "ls /var"`, AccountTransition{Kind: AccountTransitionInline, User: "postgres", Method: "su"}},

		{"sudo -i", "sudo -i", AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "sudo"}},
		{"sudo -i u", "sudo -i -u postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "sudo"}},
		{"sudo su -", "sudo su -", AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "sudo"}},
		{"sudo su - postgres", "sudo su - postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "sudo"}},
		{"sudo -u inline", `sudo -u postgres psql -c "SELECT 1"`, AccountTransition{Kind: AccountTransitionInline, User: "postgres", Method: "sudo"}},
		{"sudo plain cmd", "sudo systemctl status sshd", AccountTransition{Kind: AccountTransitionInline, User: "root", Method: "sudo"}},
		{"sudo --user= inline", "sudo --user=oracle whoami", AccountTransition{Kind: AccountTransitionInline, User: "oracle", Method: "sudo"}},
		{"sudo -u alone", "sudo -u postgres", AccountTransition{Kind: AccountTransitionEnter, User: "postgres", Method: "sudo"}},

		{"exit", "exit", AccountTransition{Kind: AccountTransitionExit}},
		{"logout", "logout", AccountTransition{Kind: AccountTransitionExit}},
		{"exit with arg", "exit 0", AccountTransition{Kind: AccountTransitionNone}},

		{"chained semi", "su - postgres; whoami", AccountTransition{Kind: AccountTransitionNone}},
		{"chained pipe", "sudo cat /etc/shadow | grep root", AccountTransition{Kind: AccountTransitionNone}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAccountTransition(tc.cmd)
			if got != tc.want {
				t.Errorf("ParseAccountTransition(%q) = %+v, want %+v", tc.cmd, got, tc.want)
			}
		})
	}
}
