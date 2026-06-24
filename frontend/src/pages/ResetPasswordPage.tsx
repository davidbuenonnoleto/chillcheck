import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Snowflake } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ResetPasswordPage() {
  const navigate = useNavigate();
  const token = new URLSearchParams(window.location.search).get("token") ?? "";
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api.resetPassword(token, password);
      toast.success("Password updated — you can sign in now.");
      navigate("/login");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not reset password");
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
          <CardTitle>Set a new password</CardTitle>
          <CardDescription>Choose a new password for your account.</CardDescription>
        </CardHeader>
        <CardContent>
          {!token ? (
            <p className="text-sm text-destructive">This reset link is missing its token.</p>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="password">New password</Label>
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
                {busy ? "Saving..." : "Update password"}
              </Button>
            </form>
          )}
          <Link to="/login" className="mt-4 block text-center text-sm text-muted-foreground hover:text-foreground">
            Back to sign in
          </Link>
        </CardContent>
      </Card>
    </div>
  );
}
