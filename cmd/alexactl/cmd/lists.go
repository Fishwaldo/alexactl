// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var listsListFlag string

var listsCmd = &cobra.Command{
	Use:   "lists",
	Short: "Read and modify Amazon shopping / to-do lists",
	Long: `Surface and modify the lists tied to your Amazon account
(default shopping list + any user-named lists).

With no subcommand, prints a summary of all lists with item counts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}

		lists, err := client.Lists.Get(ctx)
		if err != nil {
			return err
		}
		for _, l := range lists {
			label := l.Name
			if label == "" {
				label = "(default shopping list)"
			}
			active, completed := 0, 0
			for _, it := range l.Items {
				if it.Completed {
					completed++
				} else {
					active++
				}
			}
			fmt.Printf("%-30s  type=%s  default=%t  items=%d (active=%d, completed=%d)\n",
				label, l.Type, l.Default, len(l.Items), active, completed)
		}
		return nil
	},
}

var listsLsCmd = &cobra.Command{
	Use:   "ls [<list-name>]",
	Short: "Show items in a list (default: the shopping list)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}

		lists, err := client.Lists.Get(ctx)
		if err != nil {
			return err
		}

		var target *alexactl.List
		wantName := ""
		if len(args) == 1 {
			wantName = args[0]
		}
		for _, l := range lists {
			if wantName == "" && l.Default {
				target = l
				break
			}
			if wantName != "" && strings.EqualFold(l.Name, wantName) {
				target = l
				break
			}
		}
		if target == nil {
			return fmt.Errorf("list %q not found", wantName)
		}

		fmt.Printf("%-40s  %-10s  %-20s  %s\n", "ID", "STATUS", "CATEGORY", "VALUE")
		for _, it := range target.Items {
			status := "active"
			if it.Completed {
				status = "done"
			}
			fmt.Printf("%-40s  %-10s  %-20s  %s\n", it.ID, status, it.Category, it.Value)
		}
		return nil
	},
}

var listsAddCmd = &cobra.Command{
	Use:   "add <value>",
	Short: "Add an item; --list defaults to the shopping list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}

		lists, err := client.Lists.Get(ctx)
		if err != nil {
			return err
		}
		target := findListByName(lists, listsListFlag)
		if target == nil {
			if listsListFlag == "" {
				return fmt.Errorf("no default shopping list found")
			}
			return fmt.Errorf("list %q not found", listsListFlag)
		}
		item, err := target.AddItem(ctx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("added: %s  (id=%s, category=%s)\n", item.Value, item.ID, item.Category)
		return nil
	},
}

// ctxAlias avoids pulling context into every closure signature line.
type ctxAlias = context.Context

var listsCompleteCmd = &cobra.Command{
	Use:   "complete <item-id>",
	Short: "Mark item complete (looked up across all lists)",
	Args:  cobra.ExactArgs(1),
	RunE: withItem(func(ctx ctxAlias, parent *alexactl.List, item *alexactl.ListItem, _ []string) error {
		return parent.Complete(ctx, item)
	}),
}

var listsUncompleteCmd = &cobra.Command{
	Use:   "uncomplete <item-id>",
	Short: "Flip a completed item back to active",
	Args:  cobra.ExactArgs(1),
	RunE: withItem(func(ctx ctxAlias, parent *alexactl.List, item *alexactl.ListItem, _ []string) error {
		return parent.Uncomplete(ctx, item)
	}),
}

var listsDeleteCmd = &cobra.Command{
	Use:   "delete <item-id>",
	Short: "Remove an item",
	Args:  cobra.ExactArgs(1),
	RunE: withItem(func(ctx ctxAlias, parent *alexactl.List, item *alexactl.ListItem, _ []string) error {
		return parent.Delete(ctx, item)
	}),
}

var listsRenameCmd = &cobra.Command{
	Use:   "rename <item-id> <new-value>",
	Short: "Rewrite an item's value",
	Args:  cobra.ExactArgs(2),
	RunE: withItem(func(ctx ctxAlias, parent *alexactl.List, item *alexactl.ListItem, args []string) error {
		return parent.Rename(ctx, item, args[1])
	}),
}

var listsShowCmd = &cobra.Command{
	Use:   "show <item-id>",
	Short: "Print the raw server dict for an item as pretty JSON",
	Long: `Dumps the underlying ServerShape() map (the unfiltered JSON
dict Amazon returned for this item) as indented JSON. Useful for
inspecting fields the typed *ListItem hides — encryptedValue, full
listItemMetadata audit trail, customerId — and for diagnosing wire
shape changes.`,
	Args: cobra.ExactArgs(1),
	RunE: withItem(func(_ ctxAlias, _ *alexactl.List, item *alexactl.ListItem, _ []string) error {
		out, err := json.MarshalIndent(item.ServerShape(), "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}),
}

// withItem builds a RunE that looks up an item by ID across all lists
// (item IDs are UUIDs, so collision-free) and hands the resolved
// (parent, item) tuple to the action. ID is always args[0].
func withItem(action func(ctx ctxAlias, parent *alexactl.List, item *alexactl.ListItem, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}

		lists, err := client.Lists.Get(ctx)
		if err != nil {
			return err
		}
		id := args[0]
		for _, l := range lists {
			for _, it := range l.Items {
				if it.ID == id {
					return action(ctx, l, it, args)
				}
			}
		}
		return fmt.Errorf("item id %q not found in any list", id)
	}
}

// findListByName returns the matching *List by case-insensitive name,
// or the default shopping list when name == "". Nil if no match.
func findListByName(lists []*alexactl.List, name string) *alexactl.List {
	for _, l := range lists {
		if name == "" && l.Default {
			return l
		}
		if name != "" && strings.EqualFold(l.Name, name) {
			return l
		}
	}
	return nil
}

func init() {
	listsAddCmd.Flags().StringVar(&listsListFlag, "list", "", "target list name (default: the shopping list)")

	listsCmd.AddCommand(listsLsCmd)
	listsCmd.AddCommand(listsAddCmd)
	listsCmd.AddCommand(listsCompleteCmd)
	listsCmd.AddCommand(listsUncompleteCmd)
	listsCmd.AddCommand(listsDeleteCmd)
	listsCmd.AddCommand(listsRenameCmd)
	listsCmd.AddCommand(listsShowCmd)
	rootCmd.AddCommand(listsCmd)
}
