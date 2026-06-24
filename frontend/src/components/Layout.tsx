import { type ReactNode } from "react";
import { Link } from "react-router-dom";
import { Snowflake, LogOut, CreditCard, Users } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import { Button } from "@/components/ui/button";
import { BillingBanner } from "@/components/BillingBanner";
import { ThemeToggle } from "@/components/ThemeToggle";

export function Layout({ children }: { children: ReactNode }) {
  const { orgName, user, logout } = useAuth();
  return (
    <div className="min-h-screen">
      <header className="border-b bg-card">
        <div className="mx-auto flex h-14 w-full max-w-[1200px] items-center justify-between px-6">
          <Link to="/" className="flex items-center gap-2 font-semibold">
            <Snowflake className="h-5 w-5 text-primary" />
            ChillCheck
          </Link>
          <div className="flex items-center gap-3 text-sm">
            {orgName && <span className="hidden text-muted-foreground sm:inline">{orgName}</span>}
            {user && <span className="text-muted-foreground">{user.name}</span>}
            <Button variant="ghost" size="sm" asChild>
              <Link to="/team">
                <Users className="h-4 w-4" />
                <span className="hidden sm:inline">Team</span>
              </Link>
            </Button>
            <Button variant="ghost" size="sm" asChild>
              <Link to="/billing">
                <CreditCard className="h-4 w-4" />
                <span className="hidden sm:inline">Billing</span>
              </Link>
            </Button>
            <Button variant="ghost" size="sm" onClick={logout}>
              <LogOut className="h-4 w-4" />
              Sign out
            </Button>
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-[1200px] space-y-4 px-6 py-8">
        <BillingBanner />
        {children}
      </main>
    </div>
  );
}
