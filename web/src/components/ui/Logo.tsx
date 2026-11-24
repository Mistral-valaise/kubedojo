import { Link } from "react-router";

export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <path d="M16 4.5 26 10.3v11.4L16 27.5 6 21.7V10.3z" fill="none" stroke="var(--accent)" strokeWidth="2.2" strokeLinejoin="round" />
      <path d="m11.3 16 3.3 3.3 6.1-6.7" fill="none" stroke="var(--ok)" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function Brand() {
  return (
    <Link to="/" className="flex shrink-0 items-center gap-2 font-semibold tracking-tight">
      <Logo className="size-7" />
      <span className="text-base">kubedojo</span>
    </Link>
  );
}
