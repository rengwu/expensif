import { useState } from 'react';
import { Panel } from './Panel';

interface NavLink {
  label: string;
  href: string;
  active: boolean;
}

interface MobileNavProps {
  links: NavLink[];
  // Less-frequent actions (Export, Preferences), pinned to the bottom of the
  // panel — the mobile equivalent of the desktop ⋮ overflow menu.
  secondary?: NavLink[];
}

export function MobileNav({ links, secondary }: MobileNavProps) {
  const [open, setOpen] = useState(false);

  return (
    <>
      {/* Hamburger */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="sm:hidden p-1 rounded hover:bg-gray-100 text-gray-600 transition"
        aria-label="Open menu"
        aria-expanded={open}
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          className="h-6 w-6"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 6h16M4 12h16M4 18h16"
          />
        </svg>
      </button>

      <Panel isOpen={open} onClose={() => setOpen(false)} position="left" width="280px">
        <nav className="flex flex-col h-full">
          <div className="flex flex-col gap-1">
            {links.map((link) => (
              <a
                key={link.href}
                href={link.href}
                className={`block px-3 py-2.5 rounded-lg font-semibold text-sm transition ${
                  link.active
                    ? 'text-gray-900 bg-gray-100'
                    : 'text-gray-700 hover:bg-gray-50'
                }`}
                onClick={() => setOpen(false)}
              >
                {link.label}
              </a>
            ))}
          </div>
          {secondary && secondary.length > 0 && (
            <div className="mt-auto pt-4 pb-6 flex flex-col gap-1">
              {secondary.map((link) => (
                <a
                  key={link.href}
                  href={link.href}
                  className={`block px-3 py-2.5 rounded-lg font-semibold text-sm transition ${
                    link.active
                      ? 'text-gray-900 bg-gray-100'
                      : 'text-gray-700 hover:bg-gray-50'
                  }`}
                  onClick={() => setOpen(false)}
                >
                  {link.label}
                </a>
              ))}
            </div>
          )}
        </nav>
      </Panel>
    </>
  );
}
