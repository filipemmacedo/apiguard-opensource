import { HeadContent, Outlet, Scripts, createRootRoute } from '@tanstack/react-router'

import appCss from '../styles.css?url'

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'API Guard Dashboard' },
      { name: 'description', content: 'API Guard — monitor API usage, enforce PII and NSFW security policies, and manage tenant access from a single dashboard.' },
      { property: 'og:title', content: 'API Guard Dashboard' },
      { property: 'og:description', content: 'Monitor API usage, enforce security policies, and manage tenant access.' },
      { property: 'og:type', content: 'website' },
    ],
    links: [
      { rel: 'icon', type: 'image/svg+xml', href: '/assets/favicons/favicon.svg' },
      { rel: 'shortcut icon', href: '/assets/favicons/favicon.svg' },
      { rel: 'manifest', href: '/manifest.json' },
      { rel: 'stylesheet', href: appCss },
    ],
  }),
  shellComponent: RootDocument,
  component: () => <Outlet />,
})

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body className="font-sans antialiased">
        {children}
        <Scripts />
      </body>
    </html>
  )
}
