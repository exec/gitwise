import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";

// Extend default schema to allow language-* classes on <code> elements
// so syntax highlighting (prismjs) can still work.
const sanitizeSchema = {
  ...defaultSchema,
  attributes: {
    ...defaultSchema.attributes,
    code: [
      ...(defaultSchema.attributes?.code ?? []),
      ["className", /^language-/],
    ],
  },
};

interface Props {
  content: string;
}

export default function Markdown({ content }: Props) {
  return (
    <div className="markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
