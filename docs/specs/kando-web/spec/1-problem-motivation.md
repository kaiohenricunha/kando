# §1 — Problem / Motivation

> Why does this exist? What's broken? Why now?

## The Question

Can I see the extent, status, and progress of my tasks, and edit, create, or
delete them, from a browser instead of only the terminal?

## Why

The TUI only works in a terminal. The user wants a clearer, at-a-glance view of
where every task stands — the full board, not just the active lane — plus the
ability to edit, create, and delete tasks, and to manage more than one board
(multiple users, multiple kanban boards).

## What

A local web page, served from localhost, that shows the same kanban board data
as the TUI: lanes, cards, status, and progress. From the page the user can edit
a card, create a new card, delete a card, and create a new board. It reads and
writes the same board files the TUI does.

## Why Now

The TUI (kando) is built and working. The user now wants a browser-based way to
do the same task management, alongside the TUI, not instead of it. Constraint
carried forward into scope and architecture: the web dashboard and the TUI must
stay feature-mirrors of each other — they must never drift apart (see §2, §4).
