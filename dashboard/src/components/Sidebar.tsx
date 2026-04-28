import { Link, useRouterState } from "@tanstack/react-router";
import {
  FileText,
  BarChart3,
  Beaker,
  KeyRound,
  ShieldCheck,
} from "lucide-react";
import { BrandLogo } from "#/components/BrandLogo";

const navGroups = [
  {
    label: undefined,
    items: [
      { to: "/api-management", label: "API Management", icon: KeyRound },
      { to: "/guardrails", label: "Guardrails", icon: ShieldCheck },
      { to: "/playground", label: "Playground", icon: Beaker },
    ],
  },
  {
    label: "Observability",
    items: [
      { to: "/", label: "Logs", icon: FileText },
      { to: "/usage", label: "Usage", icon: BarChart3 },
    ],
  },
];

export function Sidebar({ onNavigate }: { onNavigate?: () => void } = {}) {
  const routerState = useRouterState();
  const currentPath = routerState.location.pathname;

  return (
    <aside className="w-60 h-screen flex flex-col shrink-0" style={{ background: "#141414" }}>
      {/* Logo */}
      <div className="px-5 py-5">
        <BrandLogo imageClassName="h-7 w-auto" lightSurface />
      </div>

      {/* Nav */}
      <nav aria-label="Main navigation" className="flex-1 px-3 mt-2 space-y-0.5">
        {navGroups.map((group, groupIndex) => (
          <div key={groupIndex}>
            {group.label && (
              <div className="text-[10px] font-semibold tracking-widest uppercase text-[#6B6B7B] px-3 pt-6 pb-1">
                {group.label}
              </div>
            )}
            {group.items.map((item) => {
              const isActive =
                item.to === "/"
                  ? currentPath === "/"
                  : currentPath.startsWith(item.to);
              const Icon = item.icon;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  onClick={onNavigate}
                  aria-current={isActive ? "page" : undefined}
                  className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                    isActive
                      ? "text-white bg-[#00C896]"
                      : "text-[#D4D4DC] hover:text-white hover:bg-[#252525]"
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  {item.label}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      {/* Footer */}
      <div className="px-5 pb-5 pt-3 border-t border-[#252525]">
        <p className="text-xs text-[#6B6B7B]">API Guard</p>
      </div>
    </aside>
  );
}
