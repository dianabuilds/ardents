package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func addressBookCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		addressBookList(args[1:])
	case "add":
		addressBookAdd(args[1:])
	case "export":
		addressBookExport(args[1:])
	case "import":
		addressBookImport(args[1:])
	default:
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
}

func addressBookList(args []string) {
	fs := flag.NewFlagSet("addressbook list", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func addressBookAdd(args []string) {
	fs := flag.NewFlagSet("addressbook add", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	alias := fs.String("alias", "", "alias (required)")
	identityID := fs.String("identity", "", "identity_id (did:key:...) required")
	trust := fs.String("trust", "trusted", "trusted|untrusted")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}

	if *alias == "" || *identityID == "" {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	if err := ids.ValidateAlias(*alias); err != nil {
		fatal(err)
	}
	if err := ids.ValidateIdentityID(*identityID); err != nil {
		fatal(err)
	}
	if *trust != "trusted" && *trust != "untrusted" {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	entry := addressbook.Entry{
		Alias:       *alias,
		TargetType:  "identity",
		TargetID:    *identityID,
		Source:      "self",
		Trust:       *trust,
		Note:        *note,
		CreatedAtMs: timeutil.NowUnixMs(),
	}
	book.Entries = append(book.Entries, entry)
	book.UpdatedAtMs = timeutil.NowUnixMs()
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook entry added")
}

func addressBookExport(args []string) {
	fs := flag.NewFlagSet("addressbook export", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	out := fs.String("out", "addressbook.bundle.cbor", "output file")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}

	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	id, err := identity.LoadOrCreate(dirs.IdentityDir())
	if err != nil {
		fatal(err)
	}
	node, err := book.ExportBundle(id)
	if err != nil {
		fatal(err)
	}
	data, err := contentnode.Encode(node)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle exported:", *out)
}

func addressBookImport(args []string) {
	fs := flag.NewFlagSet("addressbook import", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	in := fs.String("in", "", "input bundle file (required)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}
	if *in == "" {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	var node contentnode.Node
	if err := contentnode.Decode(data, &node); err != nil {
		fatal(err)
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	book, err = book.ImportBundle(node, timeutil.NowUnixMs())
	if err != nil {
		fatal(err)
	}
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle imported")
}
