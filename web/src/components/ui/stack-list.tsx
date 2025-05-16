"use client";

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { RefreshCw, ExternalLink, Server, Package, ArrowUp, ArrowDown, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

interface StackWithStatus {
  Name: string;
  Path: string;
  ServerName: string;
  IsRemote: boolean;
  status: string;
}

function StackList() {
  const [stacks, setStacks] = useState<StackWithStatus[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [remoteLoading, setRemoteLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState<boolean>(false);
  const [streamedOutput, setStreamedOutput] = useState<string>('');
  const [runningCommand, setRunningCommand] = useState<string | null>(null);
  const [currentStack, setCurrentStack] = useState<StackWithStatus | null>(null);
  const [isAlertDialogOpen, setIsAlertDialogOpen] = useState<boolean>(false);
  const [pendingStack, setPendingStack] = useState<StackWithStatus | null>(null);
  const [pendingAction, setPendingAction] = useState<'up' | 'down' | 'pull' | 'refresh' | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const fetchLocalStacks = async () => {
    try {
      const response = await fetch('/api/stacks/local');
      if (!response.ok) {
        throw new Error(`HTTP error fetching local stacks! status: ${response.status}`);
      }
      const localStacks: StackWithStatus[] = await response.json();
      setStacks(currentStacks => {
        // Keep any existing remote stacks, replace local ones
        const remoteStacks = currentStacks.filter(s => s.IsRemote);
        return [...localStacks, ...remoteStacks];
      });
    } catch (err) {
      console.error('Failed to fetch local stacks:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch local stacks');
      return false;
    }
    return true;
  };

  const fetchRemoteStacks = async () => {
    try {
      setRemoteLoading(true);
      const sshHostsResponse = await fetch('/api/ssh/hosts');
      if (!sshHostsResponse.ok) {
        throw new Error(`HTTP error fetching SSH hosts! status: ${sshHostsResponse.status}`);
      }

      const sshHosts: { Name: string }[] = await sshHostsResponse.json();
      
      // Start all fetches in parallel
      const remotePromises = sshHosts.map(host => {
        return fetch(`/api/ssh/hosts/${host.Name}/stacks`)
          .then(async response => {
            if (!response.ok) {
              throw new Error(`HTTP error: ${response.status}`);
            }
            const remoteStacks: StackWithStatus[] = await response.json();
            // Update stacks immediately for this host
            setStacks(currentStacks => {
              // Remove any existing stacks for this host
              const otherStacks = currentStacks.filter(s => s.ServerName !== host.Name);
              return [...otherStacks, ...remoteStacks];
            });
          })
          .catch(hostErr => {
            console.error(`Failed to fetch stacks for host ${host.Name}:`, hostErr);
            setStacks(currentStacks => {
              // Remove any existing stacks for this host
              const otherStacks = currentStacks.filter(s => s.ServerName !== host.Name);
              return [...otherStacks, {
                Name: `Error: ${host.Name} stacks unavailable`,
                Path: '',
                ServerName: host.Name,
                IsRemote: true,
                status: 'Load Error',
              }];
            });
          });
      });

      // Wait for all remote fetches to complete
      await Promise.allSettled(remotePromises);
    } catch (err) {
      console.error('Failed to fetch remote hosts:', err);
      setError(prev => {
        const errMsg = err instanceof Error ? err.message : 'Failed to fetch remote hosts';
        return prev ? `${prev}. ${errMsg}` : errMsg;
      });
    } finally {
      setRemoteLoading(false);
    }
  };

  const fetchAllStacks = useCallback(async () => {
    setLoading(true);
    setError(null);
    setStacks([]);

    try {
      // First load local stacks
      const localSuccess = await fetchLocalStacks();
      // Then start loading remote stacks
      if (localSuccess) {
        fetchRemoteStacks();
      }
    } finally {
      // We can show the table once local stacks are loaded
      setLoading(false);
    }
  }, []);

  const updateStackStatus = async (stack: StackWithStatus) => {
    try {
      const response = await fetch(stack.ServerName === 'local' 
        ? `/api/stacks/local/${stack.Name}/status`
        : `/api/ssh/hosts/${stack.ServerName}/stacks/${stack.Name}/status`);
      if (response.ok) {
        const updatedStatus = await response.json();
        setStacks(prevStacks => prevStacks.map(s => 
          s.Name === stack.Name && s.ServerName === stack.ServerName
            ? { ...s, status: updatedStatus.status }
            : s
        ));
      }
    } catch (err) {
      console.error('Failed to update stack status:', err);
    }
  };

  const executeStackAction = (stack: StackWithStatus, action: 'up' | 'down' | 'pull' | 'refresh') => {
    setCurrentStack(stack);
    setIsDialogOpen(true);
    setStreamedOutput('');
    setRunningCommand(`${stack.ServerName}:${stack.Name}:${action}`);

    // Clean up any existing EventSource
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    // For non-refresh actions, initiate the action with a POST request first
    if (action !== 'refresh') {
      fetch(`/api/run/stack/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: stack.Name, serverName: stack.ServerName }),
      }).catch(err => {
        console.error(`Failed to start ${action} action:`, err);
        setStreamedOutput(prev => prev + `\nError: ${err.message}\n`);
        setRunningCommand(null);
        return;
      });
    }

    // Use the appropriate streaming endpoint based on the action
    const streamUrl = `/api/run/stack/${action}/stream?name=${stack.Name}&serverName=${stack.ServerName}`;

    const eventSource = new EventSource(streamUrl);
    eventSourceRef.current = eventSource;

    let lastLine = '';

    const handleStreamOutput = (event: MessageEvent) => {
      // Split on newlines and filter out empty lines
      const lines = event.data.split('\\n').filter(Boolean) as string[];
      
      setStreamedOutput(prevOutput => {
        // Filter out lines that are the same as the last line
        const newLines = lines.filter((line: string) => line !== lastLine);
        if (newLines.length > 0) {
          lastLine = newLines[newLines.length - 1];
          return prevOutput + newLines.join('\n') + '\n';
        }
        return prevOutput;
      });
    };

    eventSource.addEventListener('stdout', handleStreamOutput);
    eventSource.addEventListener('stderr', handleStreamOutput);

    eventSource.addEventListener('step', (event: MessageEvent) => {
      setStreamedOutput(prevOutput => prevOutput + `--- ${event.data} ---\n`);
    });

    eventSource.addEventListener('error', (event: Event) => {
      console.error('SSE Error:', event);
      setStreamedOutput(prevOutput => prevOutput + `\nError occurred during streaming. Check console for details.\n`);
      eventSource.close();
      setRunningCommand(null);
      eventSourceRef.current = null;
    });

    eventSource.addEventListener('done', async (event: MessageEvent) => {
      setStreamedOutput(prevOutput => prevOutput + `\n${event.data}\n`);
      eventSource.close();
      eventSourceRef.current = null;
      setRunningCommand(null);
      
      // Update the stack's status after any action completes
      await updateStackStatus(stack);
    });
  };

  const confirmStackAction = (stack: StackWithStatus, action: 'up' | 'down' | 'pull' | 'refresh') => {
    setPendingStack(stack);
    setPendingAction(action);
    setIsAlertDialogOpen(true);
  };
  
  const handleActionConfirmed = () => {
    if (pendingStack && pendingAction) {
      executeStackAction(pendingStack, pendingAction);
    }
    setIsAlertDialogOpen(false);
  };

  useEffect(() => {
    fetchAllStacks();
  }, [fetchAllStacks]);

  // Clean up EventSource on unmount or when dialog closes
  useEffect(() => {
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    };
  }, []);

  // Group stacks by server
  const groupedStacks = React.useMemo(() => {
    const groups: Record<string, StackWithStatus[]> = {};
    
    stacks.forEach(stack => {
      if (!groups[stack.ServerName]) {
        groups[stack.ServerName] = [];
      }
      groups[stack.ServerName].push(stack);
    });
    
    // Sort servers to ensure consistent order with local first
    return Object.entries(groups).sort(([a], [b]) => {
      if (a === 'local') return -1;
      if (b === 'local') return 1;
      return a.localeCompare(b);
    });
  }, [stacks]);

  if (loading && stacks.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <Spinner size="lg" className="mb-4" />
        <p className="text-foreground text-lg">Discovering stacks...</p>
      </div>
    );
  }

  return (
    <div className="text-foreground">
      <div className="flex justify-between items-center mb-4">
        <div className="flex items-center gap-4">
          <h3 className="text-lg font-semibold">
            Discovered Stacks
            {remoteLoading && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                <Spinner size="sm" text="Discovering remote stacks..." />
              </span>
            )}
          </h3>
        </div>
        <Button onClick={fetchAllStacks} disabled={loading || runningCommand !== null}>
          {loading ? <Spinner size="sm" text="Refreshing List..." /> : 'Refresh List'}
        </Button>
      </div>
      {error && (
        <div className="mb-4 p-4 text-sm border rounded-md bg-destructive/10 text-destructive border-destructive/20">
          Warning: {error}
        </div>
      )}

      {groupedStacks.map(([serverName, serverStacks]) => (
        <div key={serverName} className="mb-6">
          <div className="flex items-center gap-2 mb-3">
            <Server className="h-5 w-5 text-primary" />
            <h4 className="text-md font-medium">{serverName}</h4>
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
            {serverStacks.map((stack) => {
              const commandIdentifier = `${stack.ServerName}:${stack.Name}:refresh`;
              return (
                <Card 
                  key={`${stack.ServerName}:${stack.Name}`}
                  className="relative group hover:shadow-lg transition-shadow flex flex-col pb-0 h-auto"
                >
                  <CardHeader className="py-1 px-3 mb-0 pb-1.5">
                    <CardTitle className="flex items-start justify-between">
                      <div className="flex items-center gap-2 truncate">
                        <Package className="h-5 w-5 text-primary flex-shrink-0" />
                        <span className="truncate font-medium text-sm" title={stack.Name}>{stack.Name}</span>
                      </div>
                      <Badge 
                        variant={
                          stack.status === "UP" ? "default" :
                          stack.status === "DOWN" ? "secondary" :
                          stack.status === "ERROR" || stack.status === "Load Error" ? "destructive" :
                          stack.status === "loading..." ? "outline" :
                          "secondary"
                        }
                        className={
                          stack.status === "PARTIAL" ? "bg-yellow-500 hover:bg-yellow-500/90" :
                          undefined
                        }
                      >
                        {stack.status}
                      </Badge>
                    </CardTitle>
                  </CardHeader>
                  <div className="flex flex-col bg-muted/40 rounded-b-lg mt-0">
                    <div className="border-t border-border w-full"></div>
                    <div className="flex items-center justify-center gap-1 py-1.25 px-2">
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 rounded-lg bg-background hover:bg-background/80"
                        onClick={() => confirmStackAction(stack, 'up')}
                        disabled={runningCommand !== null}
                        title="Start stack"
                      >
                        <ArrowUp className="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 rounded-lg bg-background hover:bg-background/80"
                        onClick={() => confirmStackAction(stack, 'down')}
                        disabled={runningCommand !== null}
                        title="Stop stack"
                      >
                        <ArrowDown className="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 rounded-lg bg-background hover:bg-background/80"
                        onClick={() => confirmStackAction(stack, 'pull')}
                        disabled={runningCommand !== null}
                        title="Pull latest updates"
                      >
                        <Download className="h-3 w-3" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 rounded-lg bg-background hover:bg-background/80"
                        onClick={() => confirmStackAction(stack, 'refresh')}
                        disabled={runningCommand === commandIdentifier || (runningCommand !== null && runningCommand !== commandIdentifier)}
                        title="Refresh stack"
                      >
                        <RefreshCw className={`h-3 w-3 ${runningCommand === commandIdentifier ? 'animate-spin' : ''}`} />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 rounded-lg bg-background hover:bg-background/80"
                        title="View details (coming soon)"
                      >
                        <ExternalLink className="h-3 w-3" />
                      </Button>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>
        </div>
      ))}

      <Dialog open={isDialogOpen} onOpenChange={(open) => {
        setIsDialogOpen(open);
        if (!open && eventSourceRef.current) {
          eventSourceRef.current.close();
          eventSourceRef.current = null;
          setRunningCommand(null);
        }
      }}>
        <DialogContent className="sm:max-w-[800px] bg-background">
          <DialogHeader>
            <DialogTitle>
              {currentStack ? 
                (() => {
                  const action = runningCommand?.split(':').pop() || '';
                  const formattedAction = action.charAt(0).toUpperCase() + action.slice(1);
                  return `${formattedAction} Stack: ${currentStack.Name} on ${currentStack.ServerName}`;
                })() : 'Running Command'}
            </DialogTitle>
            <div className="mt-4">
              <div className="relative">
                <div className="bg-muted p-4 rounded-md overflow-hidden ring-1 ring-border">
                  <pre className="overflow-auto whitespace-pre text-sm max-h-[400px] font-mono leading-relaxed text-foreground" style={{ maxWidth: '100%', wordWrap: 'break-word' }}>
                    {streamedOutput || 'Waiting for output...'}
                  </pre>
                </div>
              </div>
            </div>
          </DialogHeader>
        </DialogContent>
      </Dialog>

      <AlertDialog open={isAlertDialogOpen} onOpenChange={setIsAlertDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {(() => {
                const actionName = pendingAction ? pendingAction.charAt(0).toUpperCase() + pendingAction.slice(1) : '';
                return `${actionName} Stack`;
              })()}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {pendingStack && pendingAction && (
                <>
                  Are you sure you want to {pendingAction} <strong>{pendingStack.Name}</strong> on <strong>{pendingStack.ServerName}</strong>?
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction 
              onClick={handleActionConfirmed}
              className={pendingAction === 'down' ? 'bg-destructive hover:bg-destructive/90 text-destructive-foreground' : ''}
            >
              Continue
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
};

export default StackList;