import { useState } from "react";
import { Link } from "react-router-dom";
import { Plus, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { useLocations, useCreateLocation } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { OnboardingCard } from "@/components/OnboardingCard";
import { IntegrityBadge } from "@/components/IntegrityBadge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function DashboardPage() {
  const { data: locations, isLoading } = useLocations();

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Locations</h1>
          <p className="text-sm text-muted-foreground">Pick a location to log temperatures and pull reports.</p>
          <div className="mt-1">
            <IntegrityBadge />
          </div>
        </div>
        <AddLocationDialog />
      </div>

      {isLoading && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="flex items-center justify-between py-5">
                <div className="space-y-2">
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-3 w-16" />
                </div>
                <Skeleton className="h-5 w-5 rounded-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {locations && <OnboardingCard locations={locations} />}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {locations?.map((loc) => (
          <Link key={loc.id} to={`/locations/${loc.id}`}>
            <Card className="transition-colors hover:border-primary/50">
              <CardContent className="flex items-center justify-between py-5">
                <div>
                  <div className="font-medium">{loc.name}</div>
                  <div className="text-sm text-muted-foreground">
                    {loc.unit_count} {loc.unit_count === 1 ? "unit" : "units"}
                  </div>
                </div>
                <ChevronRight className="h-5 w-5 text-muted-foreground" />
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}

function AddLocationDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const create = useCreateLocation();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await create.mutateAsync({ name });
      toast.success("Location added");
      setName("");
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not add location");
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Plus className="h-4 w-4" />
          Add location
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a location</DialogTitle>
          <DialogDescription>Each restaurant or kitchen is one location.</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="loc-name">Location name</Label>
            <Input id="loc-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Main Street Kitchen" required />
          </div>
          <DialogFooter>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Adding..." : "Add location"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
