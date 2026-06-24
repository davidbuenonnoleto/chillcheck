import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Snowflake } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type InviteInfo } from "@/lib/api";
import { useAuth } from "@/auth/AuthContext";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function AcceptInvitePage() {
  const navigate = useNavigate();
  const { adoptToken } = useAuth();
  const token = new URLSearchParams(window.location.search).get("token") ?? "";

  const [info, setInfo] = useState<InviteInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!token) {
      setError("This invite link is missing its token.");
      return;
    }
    api
      .getInvite(token)
      .then(setInfo)
      .catch((err) => setError(err instanceof ApiError ? err.message : "This invite is invalid or has expired."));
  }, [token]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await api.acceptInvite({ token, name, password });
      await adoptToken(res.token);
      navigate("/");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not create your account");
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
          <CardTitle>{info ? `Join ${info.org_name}` : "Accept invite"}</CardTitle>
          <CardDescription>
            {info ? `Setting up the account for ${info.email}.` : "Set up your account."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {error ? (
            <p className="text-sm text-destructive">{error}</p>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="name">Your name</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="password">Choose a password</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                />
              </div>
              <Button type="submit" className="w-full" disabled={busy || !info}>
                {busy ? "Creating..." : "Join team"}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
