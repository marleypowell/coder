import { Popover } from "@material-ui/core"
import CircularProgress from "@material-ui/core/CircularProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { watchAgentMetadata } from "api/api"
import { WorkspaceAgent, WorkspaceAgentMetadata } from "api/typesGenerated"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Stack } from "components/Stack/Stack"
import {
  HelpPopover,
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import dayjs from "dayjs"
import {
  createContext,
  FC,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react"

export const WatchAgentMetadataContext = createContext(watchAgentMetadata)

const MetadataItem: FC<{ item: WorkspaceAgentMetadata }> = ({ item }) => {
  const styles = useStyles()

  const [isOpen, setIsOpen] = useState(false)

  const anchorRef = useRef<HTMLDivElement>(null)

  if (item.result === undefined) {
    throw new Error("Metadata item result is undefined")
  }
  if (item.description === undefined) {
    throw new Error("Metadata item description is undefined")
  }

  const staleThreshold = Math.max(
    item.description.interval + item.description.timeout * 2,
    5,
  )

  const isStale = item.result.age > staleThreshold

  // Stale data is as good as no data. Plus, we want to build confidence in our
  // users that what's shown is real. If times aren't correctly synced this
  // could be buggy. But, how common is that anyways?
  const value = isStale ? (
    <CircularProgress size={12} />
  ) : (
    <div
      className={
        styles.metadataValue +
        " " +
        (item.result.error.length === 0
          ? styles.metadataValueSuccess
          : styles.metadataValueError)
      }
    >
      {item.result.value}
    </div>
  )

  const updatesInSeconds = -(item.description.interval - item.result.age)

  return (
    <div
      className={styles.metadata}
      onMouseEnter={() => setIsOpen(true)}
      onMouseLeave={() => setIsOpen(false)}
      role="presentation"
      ref={anchorRef}
    >
      <Popover
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={() => setIsOpen(false)}
        PaperProps={{
          onMouseEnter: () => setIsOpen(true),
          // onMouseLeave: () => setIsOpen(false),
        }}
        style={{
          width: "auto",
        }}
      >
        <HelpTooltipTitle>{item.description.display_name}</HelpTooltipTitle>
        <HelpTooltipText>
          This item was collected{" "}
          {dayjs.duration(item.result.age, "s").humanize()} ago and will be
          updated in{" "}
          {dayjs.duration(Math.min(updatesInSeconds, 0), "s").humanize()}.
        </HelpTooltipText>
        {isStale ? (
          <HelpTooltipText>
            This item is now stale because the agent hasn{"'"}t reported a new
            value in {dayjs.duration(item.result.age, "s").humanize()}.
          </HelpTooltipText>
        ) : (
          <></>
        )}
        <HelpTooltipText>
          This item is collected by running the following command:
          <CodeExample code={item.description.script}></CodeExample>
        </HelpTooltipText>
      </Popover>
      <div className={styles.metadataLabel}>
        {item.description.display_name}
      </div>
      {value}
    </div>
  )
}

export interface AgentMetadataViewProps {
  metadata: WorkspaceAgentMetadata[]
}

export const AgentMetadataView: FC<AgentMetadataViewProps> = ({ metadata }) => {
  const styles = useStyles()
  if (metadata.length === 0) {
    return <></>
  }
  return (
    <Stack alignItems="flex-start" direction="row" spacing={5}>
      <div className={styles.metadataHeader}>
        {metadata.map((m) => {
          if (m.description === undefined) {
            throw new Error("Metadata item description is undefined")
          }
          return <MetadataItem key={m.description.key} item={m} />
        })}
      </div>
    </Stack>
  )
}

export const AgentMetadata: FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const [metadata, setMetadata] = useState<
    WorkspaceAgentMetadata[] | undefined
  >(undefined)

  const watchAgentMetadata = useContext(WatchAgentMetadataContext)

  useEffect(() => {
    const source = watchAgentMetadata(agent.id)

    source.onerror = (e) => {
      console.error("received error in watch stream", e)
    }
    source.addEventListener("data", (e) => {
      const data = JSON.parse(e.data)
      setMetadata(data)
    })
    return () => {
      source.close()
    }
  }, [agent.id, watchAgentMetadata])

  if (metadata === undefined) {
    return <CircularProgress size={16} />
  }

  return <AgentMetadataView metadata={metadata} />
}

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const useStyles = makeStyles((theme) => ({
  metadataHeader: {
    display: "grid",
    gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
    gap: theme.spacing(5),
    rowGap: theme.spacing(3),
  },

  metadata: {
    fontSize: 16,
  },

  metadataLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    fontWeight: "bold",
  },

  metadataValue: {
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },

  metadataValueSuccess: {
    color: theme.palette.success.light,
  },
  metadataValueError: {
    color: theme.palette.error.main,
  },
}))
