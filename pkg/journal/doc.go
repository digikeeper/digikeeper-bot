// Package journal is a client for the Digikeeper event-journal service
// (github.com/digikeeper/digikeeper-log). It appends user-meaningful domain
// events — structured records the user deliberately wants to keep (what
// happened, when, tagged, with arbitrary data) — to the journal over HTTP.
//
// This package is for journaling user events, not for shipping application
// diagnostics.
//
// Typical use:
//
//	cli, err := journal.New(ctx, cfg)
//	if err != nil {
//		return err
//	}
//	defer cli.Close()
//
//	res, err := cli.Append(ctx, journal.NewNoteAddedEvent(userID, "buy milk"))
//	if err != nil {
//		return err
//	}
//	_ = res.ID
package journal
