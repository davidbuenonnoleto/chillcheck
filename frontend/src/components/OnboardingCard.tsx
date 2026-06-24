import { Link } from "react-router-dom";
import { CheckCircle2, Circle, MapPin, Thermometer, Radio } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { Location } from "@/lib/api";

export function OnboardingCard({ locations }: { locations: Location[] }) {
  const hasLocation = locations.length > 0;
  const hasUnit = locations.some((l) => l.unit_count > 0);
  if (hasLocation && hasUnit) return null; // setup complete — hide

  const first = locations[0];
  const steps = [
    { done: hasLocation, Icon: MapPin, label: "Add your first location", hint: "Use the Add location button above." },
    {
      done: hasUnit,
      Icon: Thermometer,
      label: "Add a unit to monitor",
      hint: first ? "Open a location, then Add unit." : "Add a location first.",
    },
    {
      done: false,
      Icon: Radio,
      label: "Log a reading or connect a sensor",
      hint: "Log temps by hand, or set up a gateway to capture them automatically.",
    },
  ];

  return (
    <Card className="border-primary/30 bg-accent/40">
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Get set up</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {steps.map((s) => (
          <div key={s.label} className="flex items-start gap-2 text-sm">
            {s.done ? (
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-ok" />
            ) : (
              <Circle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
            )}
            <div>
              <span className={s.done ? "text-muted-foreground line-through" : "font-medium"}>{s.label}</span>
              {!s.done && <p className="text-xs text-muted-foreground">{s.hint}</p>}
            </div>
          </div>
        ))}
        {first && !hasUnit && (
          <Link
            to={`/locations/${first.id}`}
            className="inline-block pt-1 text-sm font-medium text-primary underline underline-offset-4"
          >
            Go to {first.name}
          </Link>
        )}
      </CardContent>
    </Card>
  );
}
