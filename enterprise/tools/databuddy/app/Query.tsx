import { stringify as csvStringify } from "csv-stringify/browser/esm";
import Long from "long";
import { ArrowDownToLine, ChartLine, CircleHelp, Code, Copy, PlayCircle, Save, Sheet } from "lucide-react";
import React from "react";
import { useRoute } from "react-router5";
import { v4 as uuidv4 } from "uuid";
import { io } from "../proto/databuddy_ts_proto";
import { executeQuery, getQuery, saveQuery } from "./api";
import ChartGrid from "./ChartGrid";
import Panel from "./components/Panel";
import Editor from "./components/Editor";
import router from "./router";
import Table from "./Table";
import toast from "./toast";
import {
  Comparator,
  compareLongs,
  compareNumbers,
  compareStringRepresentations,
  compareStrings,
  reverseComparator,
} from "../../../../app/util/sort";
import { getColumnModel } from "./model";

const NEW_QUERY_TEMPLATE = `-- name: Untitled-${Math.floor(Math.random() * 100_000_000)}
`;

export default function Query() {
  const { route } = useRoute();
  const queryId = route.params["id"] === "new" ? null : route.params["id"];

  const [charts, setCharts] = React.useState<io.buildbuddy.databuddy.Chart[] | null>(null);
  // TODO: handle parseError
  const [content, setContent] = React.useState(queryId ? null : NEW_QUERY_TEMPLATE);
  const [lastSavedContent, setLastSavedContent] = React.useState<string | null>(null);
  const [response, setResponse] = React.useState<io.buildbuddy.databuddy.QueryResult | null>(null);

  // Load query
  React.useEffect(() => {
    if (!queryId) return;
    const op = getQuery(queryId);
    op.then((response) => {
      setCharts(response.charts ?? []);
      setContent(response.sql ?? "");
      setLastSavedContent(response.sql ?? "");
      setResponse(response.cachedResult ?? null);
      if (response.parseError) {
        console.warn("Failed to parse config:", response.parseError);
      }
    });
    return () => {
      op.cancel();
    };
  }, [queryId]);

  const onSave = React.useCallback(() => {
    const id = queryId ?? uuidv4();
    const op = saveQuery(id, content ?? "");
    // TODO: abort pending save if called again, or (better yet?) add client
    // timestamp and have server respect it.
    // TODO: show success banner?
    toast("Loading", { status: "loading" });
    op.then((response) => {
      toast("Saved");
      setLastSavedContent(content);
      setCharts(response.charts ?? []);
      if (response.parseError) {
        console.warn("Failed to parse config:", response.parseError);
      }
    }).catch((error) => {
      toast(String(error), { status: "error" });
    });
    if (!queryId) {
      router.navigate("query", { id }, { replace: true });
    }
  }, [content, queryId]);

  const onExecute = React.useCallback(() => {
    const id = queryId ?? uuidv4();
    toast("Executing", { status: "loading" });
    const op = executeQuery(id, content ?? "");
    op.then((response) => {
      toast(null);
      setLastSavedContent(content);
      setResponse(response.result ?? null);
      setCharts(response.charts ?? []);
      if (response.parseError) {
        console.warn("Failed to parse config:", response.parseError);
      }
    }).catch((error) => {
      toast(String(error), { status: "error" });
    });
    if (!queryId) {
      router.navigate("query", { id }, { replace: true });
    }
  }, [content, queryId]);

  return (
    <div className="query-page">
      {content !== null && (
        <Panel
          icon={<Code />}
          title="Query"
          info={content === lastSavedContent ? null : "•"}
          controls={
            <>
              <button className="icon-button" title="Save query" onClick={onSave}>
                <Save />
              </button>
              <button className="icon-button" title="Save and run query" onClick={onExecute}>
                <PlayCircle className="icon-green" />
              </button>
            </>
          }>
          <Editor language="sql" content={content ?? ""} onSave={onSave} onChange={setContent} onExecute={onExecute} />
        </Panel>
      )}
      {response && <QueryResultView charts={charts ?? []} result={response} />}
    </div>
  );
}

type QueryResultViewProps = {
  charts: io.buildbuddy.databuddy.IChart[];
  result: io.buildbuddy.databuddy.IQueryResult;
};

function QueryResultView({ charts, result }: QueryResultViewProps) {
  const { route } = useRoute();

  const onSortChange = React.useCallback(
    (sort: string | undefined, sortDir: "asc" | "desc" | undefined) => {
      router.navigate(
        "query",
        {
          ...route.params,
          sort,
          sortDir,
        },
        { replace: true }
      );
    },
    [route.params]
  );

  const columns = React.useMemo(() => {
    return (result.columns ?? []).map(getColumnModel);
  }, [result.columns]);

  // Restructure data to be row-oriented
  const rows = React.useMemo(() => {
    const rows = [];

    const rowCount = columns[0]?.data.length;
    for (let i = 0; i < rowCount; i++) {
      const row = columns.map((col) => col.data[i]);
      rows.push(row);
    }

    // Sort rows according to URL param
    const sortColumnIndex = columns.findIndex((col) => col.name === route.params.sort);
    if (sortColumnIndex >= 0) {
      // Convert Long column values to actual Long instances, to allow comparison.
      const columnType = columns[sortColumnIndex].clientType ?? "";
      if (columnType === "Long") {
        for (const row of rows) {
          row[sortColumnIndex] = Long.fromValue(row[sortColumnIndex]);
        }
      }
      let comparator = getComparator(columnType);
      if (route.params.sortDir === "desc") {
        comparator = reverseComparator(comparator);
      }

      rows.sort((rowA: Array<any>, rowB: Array<any>) => {
        const valA = rowA[sortColumnIndex];
        const valB = rowB[sortColumnIndex];
        return comparator(valA, valB);
      });
    }

    return rows;
  }, [route.params, columns, result]);

  const header = React.useMemo(() => {
    return columns.map((column) => column.name);
  }, [columns]);

  const onClickDownload = React.useCallback(() => {
    csvStringify(
      rows,
      {
        header: true,
        columns: header,
      },
      (err, csvContent) => {
        if (err) {
          console.error("Error generating CSV", err);
          return;
        }
        const blob = new Blob([csvContent], {
          type: "text/csv;charset=utf-8;",
        });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.setAttribute("download", "query_result.csv");
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
      }
    );
  }, [rows, header]);

  const onClickCopy = React.useCallback(() => {
    // TODO: use ref instead of querySelector
    const table = document.querySelector("table.query-result-table") as HTMLTableElement | null;
    if (!table) return;

    let markdown = "";
    let plainText = "";

    Array.from(table.rows).forEach((row, rowIndex) => {
      const cells = row.cells;
      let rowContent: string[] = [];
      let rowTexts: string[] = [];
      Array.from(cells).forEach((cell) => {
        const text = (cell as HTMLElement).innerText.trim();
        rowContent.push(text);
        rowTexts.push(text);
      });

      // Markdown format
      markdown += "| " + rowContent.join(" | ") + " |\n";

      // Tab-delimited plain text format
      plainText += rowTexts.join("\t") + "\n";

      // Add a separator after the header row for Markdown
      if (rowIndex === 0) {
        markdown += "| " + "-".repeat(rowContent.length).split("").join(" | ") + " |\n";
      }
    });

    // Use the Clipboard API to write multiple types
    const clipboardData = new ClipboardItem({
      "text/html": new Blob([table.outerHTML], { type: "text/html" }),
      "text/plain": new Blob([plainText], { type: "text/plain" }),
    });

    navigator.clipboard
      .write([clipboardData])
      .then(() => {
        console.log("Table copied with multiple formats");
      })
      .catch((err) => {
        console.error("Could not copy text: ", err);
      });
  }, []);

  const headerTooltips = React.useMemo(() => {
    return columns.map((column) => `${column.name} (${column.databaseTypeName})`);
  }, [columns]);

  // Handle error / empty state
  if (result.error) {
    return (
      <div className="query-result">
        <div className="error">{result.error}</div>
      </div>
    );
  } else if (!result.columns?.length) {
    return (
      <div className="query-result">
        <Panel icon={<Sheet />} title="Results">
          <div className="no-data">No results found.</div>
        </Panel>
      </div>
    );
  }

  return (
    <>
      {!!charts?.length && (
        <Panel icon={<ChartLine />} title="Charts">
          <div className="query-result-charts">
            {charts.map((chart, i) => (
              <ChartGrid key={i} chart={chart} columns={columns} />
            ))}
          </div>
        </Panel>
      )}
      <Panel
        icon={<Sheet />}
        title="Results"
        className="query-result-table-panel"
        controls={
          <>
            <button className="icon-button copy-button" onClick={onClickCopy}>
              <Copy />
            </button>
            <button className="icon-button download-button csv-download" onMouseDown={onClickDownload}>
              <ArrowDownToLine />
            </button>
          </>
        }>
        <Table
          header={header}
          headerTooltips={headerTooltips}
          rows={rows}
          sort={route.params.sort}
          sortDir={route.params.sortDir}
          onSortChange={onSortChange}
        />
      </Panel>
    </>
  );
}

function getComparator(type: string): Comparator<any> {
  switch (type) {
    case "String":
      return compareStrings;
    case "Number":
      return compareNumbers;
    case "Long":
      return compareLongs;
    default:
      return compareStringRepresentations;
  }
}
