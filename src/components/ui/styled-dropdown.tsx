import * as React from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { ChevronDownIcon } from "lucide-react";
import { cn } from "@/lib/utils";

interface StyledDropdownProps {
  value?: string;
  onValueChange?: (value: string) => void;
  placeholder?: string;
  icon?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}

export function StyledDropdown({
  value,
  onValueChange,
  placeholder,
  icon,
  children,
  className,
}: StyledDropdownProps) {
  return (
    <SelectPrimitive.Root value={value} onValueChange={onValueChange}>
      <SelectPrimitive.Trigger
        className={cn(
          "flex max-w-full items-center gap-1.5 px-3 py-1.5 rounded-full border",
          "border-border text-foreground bg-background",
          "dark:bg-secondary hover:bg-muted",
          "transition-colors duration-200",
          "text-sm font-medium",
          "focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
          "disabled:cursor-not-allowed disabled:opacity-50",
          "data-[state=open]:ring-2 data-[state=open]:ring-ring data-[state=open]:ring-offset-2",
          className
        )}
      >
        <div className="flex items-center gap-1.5 ps-1">
          {icon && <span className="icon-sm">{icon}</span>}
          <SelectPrimitive.Value placeholder={placeholder}>
            <span className="hidden sm:block max-w-[8rem] truncate md:max-w-[10rem]">
              {value || placeholder}
            </span>
          </SelectPrimitive.Value>
          <SelectPrimitive.Icon asChild>
            <ChevronDownIcon className="icon-sm opacity-50" />
          </SelectPrimitive.Icon>
        </div>
      </SelectPrimitive.Trigger>

      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          className={cn(
            "relative z-[60] max-h-96 min-w-[8rem] overflow-hidden rounded-md border",
            "bg-popover text-popover-foreground shadow-md",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
            "data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
            "data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2",
            "data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2"
          )}
          position="popper"
        >
          <SelectPrimitive.Viewport className="p-1">
            {children}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}

interface StyledDropdownItemProps {
  value: string;
  children: React.ReactNode;
  className?: string;
}

export function StyledDropdownItem({
  value,
  children,
  className,
}: StyledDropdownItemProps) {
  return (
    <SelectPrimitive.Item
      value={value}
      className={cn(
        "relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none",
        "hover:bg-accent hover:text-accent-foreground",
        "focus:bg-accent focus:text-accent-foreground",
        "data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        className
      )}
    >
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  );
}