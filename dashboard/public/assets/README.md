Static assets that are served directly by the dashboard should live here.

Structure:
- `logos/`: brand marks, product logos, tenant logos
- `images/`: generic marketing or UI-supporting images
- `favicons/`: favicon and app icon source files

Guidelines:
- Reference files from the UI with public paths such as `/assets/logos/company.svg`.
- Keep source code and static assets separate so routes and components stay focused on behavior.
- Leave existing files at `dashboard/public/*` in place while they are still referenced by the manifest or routes.
