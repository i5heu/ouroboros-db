import { marked } from 'marked';
import sanitizeHtml from 'sanitize-html';

// Configure marked once so newline behaviour matches the previous pre-wrap display.
marked.setOptions({
    gfm: true,
    breaks: true
});

const allowedTags = [
    'p',
    'br',
    'strong',
    'em',
    'b',
    'i',
    'code',
    'pre',
    'blockquote',
    'ul',
    'ol',
    'li',
    'a',
    'hr',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'table',
    'thead',
    'tbody',
    'caption',
    'colgroup',
    'tr',
    'th',
    'td'
];

const allowedAttributes: Record<string, string[]> = {
    a: ['href', 'name', 'target', 'rel', 'title'],
    code: ['class'],
    pre: ['class'],
    th: ['colspan', 'rowspan', 'align'],
    td: ['colspan', 'rowspan', 'align']
};

const allowedSchemes = ['http', 'https', 'mailto'];

export const renderMarkdown = (value: string | null | undefined): string => {
    if (!value) return '';
    const trimmed = value.trim();
    if (!trimmed) return '';
    const rendered = marked.parse(value) as string;
    return sanitizeHtml(rendered, {
        allowedTags,
        allowedAttributes,
        allowedSchemes,
        allowedSchemesAppliedToAttributes: ['href'],
        transformTags: {
            a: sanitizeHtml.simpleTransform('a', {
                target: '_blank',
                rel: 'noopener noreferrer'
            })
        },
        enforceHtmlBoundary: true
    });
};
