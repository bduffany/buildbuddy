import * as monaco from "monaco-editor";

export const configureMonaco = () => {
  self.MonacoEnvironment = {
    getWorkerUrl: function (workerId: string, label: string) {
      return "/monaco/editor.worker.js";
    },
  };

  registerSqlLanguage();
};

function registerSqlLanguage() {
  const builtInFunctions = [
    "addDays",
    "countIf",
    "greatest",
    "least",
    "now",
    "subtractDays",
    "toDateTime",
    "toStartOfDay",
    "toUnixTimestamp",

    "AVG",
    "CEIL",
    "COUNT",
    "FLOOR",
    "ROUND",
    "SUM",
  ];

  monaco.languages.register({ id: "sql" });
  monaco.languages.setMonarchTokensProvider("sql", {
    tokenizer: {
      root: [
        [
          /[A-Za-z_][\w]*/,
          {
            cases: {
              "@keywords": "keyword",
              "@builtInFunctions": "predefined",
              "@default": "identifier",
            },
          },
        ],
        [/\d+(\.\d+)?(e-?\d+)?/, "number"],
        [/[;,.]/, "delimiter"],
        [/".*?"/, "string"],
        [/'[^']*'/, "string"],
        [/`.*?`/, "string"],
        [/--.*$/, "comment"],
        [/\/\/.*$/, "comment"],
        [/<=|>=|!=|=|<|>|\+|-|\*|\//, "operator"],
      ],
    },
    keywords: [
      "AND",
      "AS",
      "ASC",
      "BY",
      "CASE",
      "CREATE",
      "DELETE",
      "DESC",
      "DISTINCT",
      "DROP",
      "ELSE",
      "END",
      "FROM",
      "GROUP",
      "INNER",
      "INSERT",
      "JOIN",
      "LEFT",
      "LIKE",
      "LIMIT",
      "NULL",
      "OR",
      "ORDER",
      "OUTER",
      "RIGHT",
      "SELECT",
      "TABLE",
      "THEN",
      "UPDATE",
      "WHEN",
      "WHERE",
      "WITH",
    ],
    builtInFunctions: builtInFunctions,
  });
  monaco.languages.setLanguageConfiguration("sql", {
    comments: {
      lineComment: "--",
    },
    brackets: [
      ["{", "}"],
      ["[", "]"],
      ["(", ")"],
    ],
    autoClosingPairs: [
      { open: "{", close: "}" },
      { open: "[", close: "]" },
      { open: "(", close: ")" },
      { open: '"', close: '"' },
      { open: "'", close: "'" },
    ],
  });
}
