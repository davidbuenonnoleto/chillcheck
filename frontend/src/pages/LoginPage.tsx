import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Snowflake } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/auth/AuthContext";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginPage() {
  const { login, register } = useAuth();
  const navigate = useNavigate();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [busy, setBusy] = useState(false);

  const [orgName, setOrgName] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      if (mode === "login") {
        await login(email, password);
      } else {
        await register({ org_name: orgName, name, email, password });
      }
      navigate("/");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="mb-1 flex items-center gap-2 text-primary">
            <Snowflake className="h-6 w-6" />
            <span className="text-lg font-semibold text-foreground">ChillCheck</span>
          </div>
          <CardTitle>{mode === "login" ? "Sign in" : "Create your account"}</CardTitle>
          <CardDescription>
            {mode === "login"
              ? "Temperature logs and inspection-ready reports."
              : "Start logging temperatures in a couple of minutes."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-4">
            {mode === "register" && (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="org">Restaurant / business name</Label>
                  <Input id="org" value={orgName} onChange={(e) => setOrgName(e.target.value)} required />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="name">Your name</Label>
                  <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
                </div>
              </>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            <Button type="submit" className="w-full" disabled={busy}>
              {busy ? "Working..." : mode === "login" ? "Sign in" : "Create account"}
            </Button>
          </form>
          <button
            className="mt-4 w-full text-center text-sm text-muted-foreground hover:text-foreground"
            onClick={() => setMode(mode === "login" ? "register" : "login")}
          >
            {mode === "login" ? "Need an account? Create one" : "Already have an account? Sign in"}
          </button>
          {mode === "login" && (
            <Link
              to="/forgot-password"
              className="mt-2 block text-center text-sm text-muted-foreground hover:text-foreground"
            >
              Forgot password?
            </Link>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
