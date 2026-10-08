package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"shelfcheck/internal/libby"
)

const libbyUsage = `usage: shelfcheck libby <command>

  link <code>        link to your Libby account with an 8-digit code from
                     Libby: Menu > Settings > Copy To Another Device
  probe [-out dir]   save raw Libby account data (sync, tags) for inspection
`

func libbyPath() string { return filepath.Join(configDir(), "libby.json") }

func libbyCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stderr, libbyUsage)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch args[0] {
	case "link":
		if len(args) != 2 {
			return errors.New("usage: shelfcheck libby link <8-digit code>")
		}
		c := libby.New("")
		id, err := c.Link(ctx, args[1])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(configDir(), 0o700); err != nil {
			return err
		}
		if err := libby.Save(libbyPath(), libby.Saved{Identity: id, Linked: time.Now()}); err != nil {
			return err
		}
		fmt.Printf("linked; saved to %s\n", libbyPath())
		return nil
	case "probe":
		return libbyProbe(ctx, args[1:])
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], libbyUsage)
}

// libbyProbe saves raw responses privately and prints only a summary.
func libbyProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("libby probe", flag.ContinueOnError)
	out := fs.String("out", filepath.Join(configDir(), "libby-probe"), "directory for raw responses")
	if err := fs.Parse(args); err != nil {
		return err
	}
	saved, err := libby.Load(libbyPath())
	if err != nil {
		return err
	}
	c := libby.New(saved.Identity)
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	write := func(name string, raw json.RawMessage) error {
		return os.WriteFile(filepath.Join(*out, name), raw, 0o600)
	}

	sync, err := c.GetRaw(ctx, c.SyncURL())
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := write("sync.json", sync); err != nil {
		return err
	}
	var s struct {
		Cards []json.RawMessage `json:"cards"`
		Loans []json.RawMessage `json:"loans"`
		Holds []json.RawMessage `json:"holds"`
	}
	json.Unmarshal(sync, &s)
	fmt.Printf("sync: %d cards, %d loans, %d holds\n", len(s.Cards), len(s.Loans), len(s.Holds))

	tagsRaw, err := c.GetRaw(ctx, c.TagsURL())
	if err != nil {
		return fmt.Errorf("tags: %w", err)
	}
	if err := write("tags.json", tagsRaw); err != nil {
		return err
	}
	var tags struct {
		Tags []struct {
			UUID          string `json:"uuid"`
			ID            string `json:"id"`
			Name          string `json:"name"`
			TotalTaggings int    `json:"totalTaggings"`
		} `json:"tags"`
	}
	json.Unmarshal(tagsRaw, &tags)
	fmt.Printf("tags: %d\n", len(tags.Tags))
	for i, t := range tags.Tags {
		id := t.UUID
		if id == "" {
			id = t.ID
		}
		fmt.Printf("  %-30q %4d titles\n", t.Name, t.TotalTaggings)
		raw, err := c.GetRaw(ctx, c.TagURL(id, t.Name, 0, 50))
		if err != nil {
			fmt.Printf("    first page: %v\n", err)
			continue
		}
		if err := write(fmt.Sprintf("tag-%02d.json", i), raw); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Printf("raw responses in %s\n", *out)
	return nil
}
