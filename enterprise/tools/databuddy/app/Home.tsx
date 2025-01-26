import React from "react";
import { getQueries, useAPI } from "./api";
import toast from "./toast";

export default function Home() {
  const { response, error, loading } = useAPI(getQueries);

  React.useEffect(() => {
    if (error) {
      toast(String(error), { status: "error" });
    } else if (loading) {
      toast("Loading", { status: "loading" });
    }
  }, [error, loading]);

  return (
    <div className="home-page">
      <h1>Queries</h1>
      <div>
        <a href="/q/new">+New query</a>
      </div>
      <div className="query-list">
        {response?.queries?.map((query) => (
          <a key={query.queryId} className="query-list-item" href={`/q/${query.queryId}`}>
            {query.name || `Untitled query ${query.queryId}`}{query.author ? ` (${query.author})` : null}
          </a>
        ))}
      </div>
    </div>
  );
}
