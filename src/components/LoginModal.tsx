import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Github } from "lucide-react";

interface LoginModalProps {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function LoginModal({ open, onOpenChange }: LoginModalProps) {
  const handleGitHubLogin = () => {
    // Redirect to GitHub OAuth endpoint
    window.location.href = "/v1/auth/github";
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} modal>
      <DialogContent className="sm:max-w-[425px]" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Login Required</DialogTitle>
          <DialogDescription>
            Please login with GitHub to continue using Catnip.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-4">
          <Button onClick={handleGitHubLogin} className="w-full" size="lg">
            <Github className="mr-2 h-5 w-5" />
            Login with GitHub
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
