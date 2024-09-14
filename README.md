# gotaskmaster

GoTaskMaster is a flexible process management tool developed using the Go language and designed to simplify task management in complex environments. It provides a careful Terminal User Interface (TUI) that allows users to easily monitor, control and manage multitasking a process

```mermaid
graph LR
    subgraph process
        NM[NewManager] --> |creates| M[Manager]
        M --> AP[AddProcess]
        M --> RP[RemoveProcess]
        M --> GP[GetProcesses]
        M --> GPr[GetProcess]
        M --> IP[initProcesses]
        M --> S[Start]
        M --> St[Stop]
        M --> MP[manageProcess]
        M --> RP[runProcess]
        M --> SLF[setupLogFile]
        M --> CLF[closeLogFile]
        M --> HO[handleOutput]
        M --> WL[writeLog]
        M --> LE[logError]
        M --> SP[StartProcess]
        M --> StP[StopProcess]
        M --> RSP[RestartProcess]
        M --> ModP[ModifyProcess]
        M --> DP[DeleteProcess]
        M --> TPS[ToggleProcessState]
        M --> GPS[GetProcessStatus]
        M --> GAPS[GetAllProcessesStatus]
        M --> UPC[UpdateProcessConfig]
        M --> CPL[CleanupProcessLogs]
        M --> RPL[RotateProcessLogs]
        M --> GPM[GetProcessMetrics]
        M --> UPM[UpdateProcessMetrics]
        M --> SMI[SetMaxInstances]
        M --> GPL[GetProcessLogs]
        M --> EPC[ExportProcessConfig]
        M --> IPC[ImportProcessConfig]
        
        P[Process] --> ELF[ensureLogFileExists]
    end

    NM --> |uses| Config
```

```mermaid
graph TB
    subgraph config
        LC[LoadConfig] --> VC[validateConfig]
        LC --> SD[setDefaults]
        LC --> GC[GetConfig]
        SC[SaveConfig] --> GC
    end

    LC --> |creates| C[Config]
    SC --> |uses| C
```

```mermaid
graph LR
    subgraph ui
        NTUI[NewTUI] --> |creates| TUI[TUI]
        TUI --> SUI[setupUI]
        TUI --> SKB[setupKeyBindings]
        TUI --> R[Run]
        TUI --> PU[periodicUpdates]
        TUI --> RPL[refreshProcessList]
        TUI --> UEC[updateErrorCount]
        TUI --> UEvC[updateEventCount]
        TUI --> SSP[startSelectedProcess]
        TUI --> StSP[stopSelectedProcess]
        TUI --> RSP[restartSelectedProcess]
        TUI --> GSP[getSelectedProcess]
        TUI --> OPS[onProcessSelected]
        TUI --> UPI[updateProcessInfo]
        TUI --> ULV[updateLogView]
        TUI --> OCE[onCommandEntered]
        TUI --> SLV[showLogView]
        TUI --> SCE[showConfigEditor]
        TUI --> SE[showError]
        TUI --> SM[showMessage]
        TUI --> SH[showHelp]
    end

    NTUI --> |uses| Manager
```




## Main features

- Multi-environment support: Compatible with various bash environments, including scientific computing environments such as Anaconda.
- Interactive TUI: Provides colorful, multi-level menu interface and intuitive operation.
- Real-time process management: start, stop, restart processes, and monitor running status in real time.
- Custom task configuration: Flexibly set the commands, parameters, environment variables and number of instances of the process.
- Log management: Automatically collect and classify the standard output and error logs of each process.
- Resource Control: Limit the maximum number of instances of each task to avoid resource overuse.
- Statistics: Provides a statistical overview of running and stopped tasks.

## Applicable scenarios

GoTaskMaster is particularly suitable for the following scenarios:

- Manage complex development environments such as data science workflows
- Monitor and maintain long-running background services
- Automated testing and continuous integration/continuous deployment (CI/CD) processes
- Manage multiple components in a distributed system

## Technical features

- Developed using Go language, with excellent performance and cross-platform compatibility
- Build a beautiful TUI based on the Bubble Tea framework
- Using coroutines to achieve efficient concurrent task management
- Modular design, easy to expand and customize
