import { createFileRoute, Outlet } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Menu } from 'lucide-react'
import { BrandLogo } from '#/components/BrandLogo'
import { Sidebar } from '#/components/Sidebar'
import { ErrorBoundary } from '#/components/ErrorBoundary'

export const Route = createFileRoute('/_layout')({
  component: LayoutComponent,
})

function LayoutComponent() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const sidebarRef = useRef<HTMLDivElement>(null)

  const closeSidebar = useCallback(() => {
    setSidebarOpen(false)
    menuButtonRef.current?.focus()
  }, [])

  useEffect(() => {
    if (sidebarOpen && sidebarRef.current) {
      const firstLink = sidebarRef.current.querySelector('a')
      firstLink?.focus()
    }
  }, [sidebarOpen])

  return (
    <div className="flex h-screen" style={{ background: '#F8F9FA' }}>
      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={closeSidebar}
        />
      )}

      {/* Sidebar — hidden on mobile, shown on lg+ */}
      <div
        ref={sidebarRef}
        className={`fixed inset-y-0 left-0 z-50 transition-transform duration-200 lg:static lg:translate-x-0 ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <Sidebar onNavigate={closeSidebar} />
      </div>

      {/* Main area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Mobile top bar with hamburger */}
        <div className="flex h-12 items-center border-b border-[#E5E7EB] bg-white px-4 lg:hidden">
          <button
            ref={menuButtonRef}
            type="button"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open menu"
            aria-expanded={sidebarOpen}
            className="flex items-center justify-center text-[#374151]"
          >
            <Menu size={20} />
          </button>
          <BrandLogo className="ml-3" imageClassName="h-6 w-auto" />
        </div>

        <main className="flex-1 overflow-auto">
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </main>
      </div>
    </div>
  )
}
