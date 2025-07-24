import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { AlertTriangle, Bot } from "lucide-react";

interface ErrorAlertProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  actionLabel?: string;
  secondaryAction?: {
    label: string;
    onClick: () => void;
    variant?: "default" | "destructive";
  };
}

export function ErrorAlert({
  open,
  onOpenChange,
  title,
  description,
  actionLabel = "OK",
  secondaryAction,
}: ErrorAlertProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive" />
            <AlertDialogTitle>{title}</AlertDialogTitle>
          </div>
          <AlertDialogDescription className="text-left whitespace-pre-wrap">
            {description}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          {secondaryAction && (
            <AlertDialogAction
              onClick={() => {
                onOpenChange(false);
                secondaryAction.onClick();
              }}
              className="bg-blue-600 hover:bg-blue-700 text-white flex items-center gap-2"
            >
              <Bot size={16} />
              {secondaryAction.label}
            </AlertDialogAction>
          )}
          <AlertDialogCancel onClick={() => onOpenChange(false)}>
            {actionLabel}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
