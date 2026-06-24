import { useState } from "react";
import { Link } from "react-router-dom";
import { Snowflake } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api.forgotPassword(email);
    } finally {
      // Always show the same confirmation — don't reveal whether the email exists.
      setSent(true);
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
          <CardTitle>Reset your password</CardTitle>
          <CardDescription>We'll email you a reset link.</CardDescription>
        </CardHeader>
        <CardContent>
          {sent ? (
            <p className="text-sm text-muted-foreground">
              If an account exists for that email, a reset link is on its way. Check your inbox.
            </p>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="email">Email</Label>
                <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
              </div>
              <Button type="submit" className="w-full" disabled={busy}>
                {busy ? "Sending..." : "Send reset link"}
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
