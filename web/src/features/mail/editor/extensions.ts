import StarterKit from '@tiptap/starter-kit'
import { Placeholder } from '@tiptap/extensions'
import { Color, FontSize, TextStyle } from '@tiptap/extension-text-style'
import { Highlight } from '@tiptap/extension-highlight'
import { TextAlign } from '@tiptap/extension-text-align'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { TableKit } from '@tiptap/extension-table'
import { Image } from '@tiptap/extension-image'

/**
 * Extended Image node that preserves `data-cid` and `data-filename` attributes
 * so C9 backend can seamlessly convert inline images into MIME CID parts.
 */
const EmailImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      'data-cid': {
        default: null,
        parseHTML: (el) => el.getAttribute('data-cid'),
        renderHTML: (attrs) => (attrs['data-cid'] ? { 'data-cid': attrs['data-cid'] } : {}),
      },
      // Original URL of an external image shown through the image proxy;
      // restored before sending.
      'data-remote-src': {
        default: null,
        parseHTML: (el) => el.getAttribute('data-remote-src'),
        renderHTML: (attrs) =>
          attrs['data-remote-src'] ? { 'data-remote-src': attrs['data-remote-src'] } : {},
      },
      'data-filename': {
        default: null,
        parseHTML: (el) => el.getAttribute('data-filename'),
        renderHTML: (attrs) =>
          attrs['data-filename'] ? { 'data-filename': attrs['data-filename'] } : {},
      },
    }
  },
})

export function createComposeExtensions(placeholder: string) {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: {
        openOnClick: false,
        autolink: true,
        HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
      },
    }),
    Placeholder.configure({
      placeholder,
    }),
    TextStyle,
    Color,
    FontSize,
    Highlight.configure({ multicolor: true }),
    TextAlign.configure({
      types: ['heading', 'paragraph'],
    }),
    TaskList,
    TaskItem.configure({
      nested: true,
    }),
    TableKit.configure({
      table: {
        resizable: false,
        HTMLAttributes: { class: 'pmail-editor-table' },
      },
    }),
    EmailImage.configure({
      allowBase64: true,
      HTMLAttributes: { class: 'pmail-editor-image' },
    }),
  ]
}
