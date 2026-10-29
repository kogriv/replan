# frozen_string_literal: true

module API
  module V3
    module WikiPages
      # Same terms as the browser: deleting a page needs manage_wiki. OpenProject
      # 17.6 folded its former separate delete permission into manage_wiki.
      class WikiPageDeleteContract < ::DeleteContract
        delete_permission :manage_wiki

        validate :no_child_pages

        private

        # The browser asks what to do with child pages. The API does not guess:
        # the client moves or deletes the children first.
        def no_child_pages
          return unless model.children.exists?

          errors.add :base, :has_child_pages,
                     message: "The page has child pages. Move or delete them first."
        end
      end
    end
  end
end
