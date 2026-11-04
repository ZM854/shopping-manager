# Shopping Manager

Shopping Manager is a web application for keeping personal shopping lists, sharing copies by link, and organizing recipes. It started as a university coursework project in backend web development.

## Features

- **Accounts.** Register with email verification, sign in, and sign out.
- **Shopping lists.** Create multiple named lists, add items with optional quantities and units, and mark items as purchased. Mark or unmark all items or clear a list in one action.
- **Sharing by link.** Share a snapshot of a list and choose whether to keep purchase marks. Signed-in recipients can import an independent copy that they can edit separately.
- **Personal recipes.** Create, view, edit, and delete recipes with a category, cooking time, ingredients, and preparation steps. Reorder preparation steps in the editor.

## Tech Stack

- **Frontend.** React, TypeScript, Vite, and CSS Modules.
- **Backend.** Go with Gin and PostgreSQL.
- **Local environment.** Docker Compose, Nginx and SeaweedFS.

## Getting Started

Install Docker with the Compose plugin and Node.js 22.21 or newer. From the repository root, run the commands for your environment.

**Windows (PowerShell)**

```powershell
.\scripts\dev.ps1 init
.\scripts\dev.ps1 up
```

**Linux or WSL (with Make)**

```sh
make init
make up
```

Open the [app](http://localhost:5173) and use [Mailpit](http://localhost:8025) to open the account activation email after registering. Compose applies database migrations on startup.

For hot reload, use `.\scripts\dev.ps1 dev` or `make dev`. To stop the built stack, run `.\scripts\dev.ps1 down -Mode built` or `make down MODE=built`.

## Project Status

The project is in active development. Recipe media is the current focus. 
