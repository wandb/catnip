import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';

interface ErrorDisplayProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  retryLabel?: string;
}

const ERROR_LOGOS = [
  'pissed@2x.png',
  'sad@2x.png',
  'suprised@2x.png',
  'suspicious@2x.png'
];

export function ErrorDisplay({ 
  title = 'Oops!', 
  message, 
  onRetry, 
  retryLabel = 'Try Again' 
}: ErrorDisplayProps) {
  const [selectedLogo, setSelectedLogo] = useState<string>('');

  useEffect(() => {
    const randomLogo = ERROR_LOGOS[Math.floor(Math.random() * ERROR_LOGOS.length)];
    setSelectedLogo(randomLogo);
  }, []);

  return (
    <div className="flex flex-col items-center justify-center min-h-[400px] p-8 text-center">
      {selectedLogo && (
        <div>
          <img 
            src={`/${selectedLogo}`} 
            alt="Error illustration" 
            className="w-64 h-64 mx-auto opacity-90"
          />
        </div>
      )}
      
      <div className="space-y-4 max-w-md">
        <h2 className="text-2xl font-semibold text-foreground">{title}</h2>
        <p className="text-muted-foreground leading-relaxed">{message}</p>
        
        {onRetry && (
          <Button 
            onClick={onRetry}
            variant="outline"
            className="mt-6"
          >
            {retryLabel}
          </Button>
        )}
      </div>
    </div>
  );
}