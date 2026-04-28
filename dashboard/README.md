API Guard Tenant Dashboard (TanStack Start)

## Getting Started

To run this application:

```bash
npm install
npm run dev
```

The app expects API Guard backend running on `http://localhost:8080`.
Vite proxy forwards `/internal/*` and `/v1/*` to the backend.

## Building For Production

To build this application for production:

```bash
npm run build
```

## Testing

This project uses [Vitest](https://vitest.dev/) for testing. You can run the tests with:

```bash
npm run test
```

## Routes

- `/` logs view
- `/usage` usage totals view
- `/playground` prompt playground with raw response + usage display

## Tests

Run:

```bash
npm run test
```
