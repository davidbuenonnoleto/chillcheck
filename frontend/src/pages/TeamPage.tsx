import { useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, UserPlus, Copy, Check, X } from "lucide-react";
import { toast } from "sonner";
import { useUsers, useInvites, useCreateInvite, useRevokeInvite } from "@/hooks/queries";
import { useAuth } from "@/auth/AuthContext";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

export function TeamPage() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const { data: users } = useUsers();
  const { data: invites } = useInvites();

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        Back
      </Link>

      <div>
        <h1 className="text-2xl font-semibold">Team</h1>
        <p className="text-sm text-muted-foreground">People who can log temperatures and view this account.</p>
      </div>

      {isAdmin && <InviteForm />}

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Members</CardTitle>
        </CardHeader>
        <CardContent className="divide-y">
          {users?.map((u) => (
            <div key={u.id} className="flex items-center justify-between py-2 text-sm">
              <div>
                <div className="font-medium">{u.name}</div>
                <div className="text-muted-foreground">{u.email}</div>
              </div>
              <Badge variant={u.role === "admin" ? "default" : "outline"}>{u.role}</Badge>
            </div>
          ))}
        </CardContent>
      </Card>

      {isAdmin && invites && invites.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Pending invites</CardTitle>
          </CardHeader>
          <CardContent className="divide-y">
            {invites.map((inv) => (
              <PendingInvite key={inv.id} id={inv.id} email={inv.email} role={inv.role} />
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function InviteForm() {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("staff");
  const [link, setLink] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const create = useCreateInvite();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await create.mutateAsync({ email, role });
      setLink(res.accept_url);
      setEmail("");
      toast.success("Invite sent");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not create invite");
    }
  }

  function copy() {
    if (!link) return;
    navigator.clipboard.writeText(link).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Invite a teammate</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
          <div className="flex-1 space-y-1.5" style={{ minWidth: 200 }}>
            <Label htmlFor="invite-email">Email</Label>
            <Input
              id="invite-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="cook@restaurant.com"
              required
            />
          </div>
          <div className="space-y-1.5">
            <Label>Role</Label>
            <Select value={role} onValueChange={setRole}>
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="staff">Staff</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button type="submit" disabled={create.isPending}>
            <UserPlus className="h-4 w-4" />
            Invite
          </Button>
        </form>

        {link && (
          <div className="space-y-1.5 rounded-md border border-accent bg-accent/40 p-3">
            <p className="text-xs text-muted-foreground">
              Invite link (also emailed if email is configured) — share it if needed:
            </p>
            <div className="flex items-center gap-2">
              <code className="flex-1 truncate rounded bg-muted px-2 py-1.5 text-xs">{link}</code>
              <Button size="icon" variant="outline" onClick={copy}>
                {copied ? <Check className="h-4 w-4 text-ok" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PendingInvite({ id, email, role }: { id: string; email: string; role: string }) {
  const revoke = useRevokeInvite();
  return (
    <div className="flex items-center justify-between py-2 text-sm">
      <div>
        <span className="font-medium">{email}</span>
        <span className="text-muted-foreground"> · {role}</span>
      </div>
      <Button
        size="sm"
        variant="ghost"
        onClick={() =>
          revoke.mutate(id, {
            onSuccess: () => toast.success("Invite revoked"),
            onError: () => toast.error("Could not revoke"),
          })
        }
      >
        <X className="h-4 w-4" />
        Revoke
      </Button>
    </div>
  );
}
